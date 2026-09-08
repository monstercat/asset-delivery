package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/marcw/cachecontrol"
	"github.com/monstercat/golib/logger"

	. "github.com/monstercat/asset-delivery"
)

type Server struct {
	logger.Logger
	FS             FileSystem
	PB             Publisher
	PermittedHosts []string
	Prefix         string
}

func (s *Server) HostPermitted(host string) bool {
	if len(s.PermittedHosts) == 0 {
		return true
	}
	for _, x := range s.PermittedHosts {
		if testHostWithPattern(strings.TrimSpace(x), host) {
			return true
		}
	}
	return false
}

// testHostWithPattern will test the hosts, allowing for a * pattern (separated by .). Note that the host should still
// contain the same # of parts. For example, *.monstercat.com will not match with beta.app.monstercat.com.
func testHostWithPattern(pattern, host string) bool {
	patternParts := strings.Split(pattern, ".")
	hostParts := strings.Split(host, ".")
	if len(hostParts) != len(patternParts) {
		return false
	}
	for i, p := range patternParts {
		curr := hostParts[i]
		if p == "*" {
			continue
		}
		if curr != p {
			return false
		}
	}
	return true
}

// TODO: generate a request id that can be passed along for all requests.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		s.Logger.Log(logger.SeverityWarning, "Request received with method "+r.Method)
		w.WriteHeader(http.StatusForbidden)
		return
	}
	opts, err := NewResizeOptionsFromQuery(r.URL.Query())
	if err != nil {
		WriteError(w, err)
		return
	}
	opts.Prefix = s.Prefix

	l := &logger.Contextual{
		Logger:  s.Logger,
		Context: opts,
	}

	if !s.HostPermitted(opts.URL.Host) {
		l.Log(logger.SeverityWarning, "Invalid host: "+opts.URL.Host)
		WriteError(w, &ParamError{Param: "url", Detail: "Host is not permitted to perform this action."})
		return
	}
	need, err := s.NeedsResizing(opts)
	if err != nil {
		if v, ok := err.(RootError); ok && v.Root() != nil {
			l.Log(logger.SeverityWarning, "Could not check needs resizing. "+err.Error()+"; "+v.Root().Error())
		} else {
			l.Log(logger.SeverityWarning, "Could not check needs resizing. "+err.Error())
		}
		WriteError(w, err)
		return
	}
	if !need {
		http.Redirect(w, r, s.FS.ObjectURL(opts.ObjectKey()), http.StatusPermanentRedirect)
		return
	}

	s.sendResize(opts.ResizeOptions, l)
	http.Redirect(w, r, opts.Location, http.StatusTemporaryRedirect)
}

// sendResize sends the resize commands quietly.
// TODO: pass in publish topic through an environment variable
func (s *Server) sendResize(opts ResizeOptions, l logger.Logger) {
	// Send resize request
	b, err := json.Marshal(opts)
	if err != nil {
		l.Log(logger.SeverityError, fmt.Sprintf("Could not marshal resize options. %s", err))
		return
	}
	l.Log(logger.SeverityInfo, "Sending resize request on "+ResizeTopic)
	if err := s.PB.Publish(ResizeTopic, b); err != nil {
		l.Log(logger.SeverityError, fmt.Sprintf("Could not send resize command. %s", err))
		return
	}
	l.Log(logger.SeverityInfo, "Resize request sent "+ResizeTopic)
}

func isExpired(info FileInfo) bool {
	control := cachecontrol.Parse(info.CacheControl())
	if control.MaxAge() <= 0 {
		return false
	}
	return time.Now().After(info.Created().Add(control.MaxAge()))
}

func (s *Server) NeedsResizing(opts ResizeOptionsProcessed) (bool, error) {
	if opts.Force {
		return true, nil
	}
	info, err := s.FS.Info(opts.ObjectKey())
	if err != nil && err != ErrNoFile {
		return false, &SystemError{RootError: err, Detail: "Could not check if image already exists."}
	} else if info != nil && !isExpired(info) {
		return false, nil
	}
	return true, nil
}

// SignSecretEnv holds the HMAC shared secret used to authenticate
// /sign requests (also held by gold-http).
const SignSecretEnv = "CDX_SIGN_SECRET"

// signHandler serves GET /sign/<cacheKey>?width=&encoding=&signature=
// &expires=. Returns 200 {url} if the resized object exists and is
// fresh, else 404. Never creates. Invalid/missing/expired auth ->
// blank 404 (reveals nothing).
//
// The returned URL is the public ObjectURL at the CacheKey-keyed path;
// the gate is that the CacheKey (and thus the object's path) is
// unguessable by anon, and /sign is the only thing that reveals it.
func (s *Server) signHandler(w http.ResponseWriter, r *http.Request) {
	cacheKey := strings.TrimPrefix(r.URL.Path, "/sign/")
	q := r.URL.Query()
	widthU, err := strconv.ParseUint(q.Get("width"), 10, 32)
	if err != nil || widthU == 0 || widthU > MaxImageDimension {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	width := uint(widthU)
	enc := q.Get("encoding")
	sig := q.Get("signature")
	expires, _ := strconv.ParseInt(q.Get("expires"), 10, 64)

	secret := []byte(os.Getenv(SignSecretEnv))
	if !verifySignSig(secret, cacheKey, width, enc, sig, expires) ||
		time.Now().Unix() > expires {
		w.WriteHeader(http.StatusNotFound) // blank 404
		return
	}

	opts := ResizeOptions{
		CacheKey: cacheKey,
		Width:    width,
		Encoding: enc,
		Prefix:   s.Prefix,
	}
	opts.PopulateHash() // hashes CacheKey

	info, err := s.FS.Info(opts.ObjectKey())
	if err == ErrNoFile || info == nil || isExpired(info) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"url": s.FS.ObjectURL(opts.ObjectKey()),
	})
}

// verifySignSig checks sig (hex) == HMAC-SHA256(secret,
// cacheKey:width:enc:expires). Covers every param a caller can
// tamper with.
func verifySignSig(secret []byte, cacheKey string, width uint,
	enc, sig string, expires int64) bool {
	if len(secret) == 0 || sig == "" {
		return false
	}
	msg := cacheKey + ":" +
		strconv.FormatUint(uint64(width), 10) + ":" +
		enc + ":" + strconv.FormatInt(expires, 10)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(msg))
	got, err := hex.DecodeString(sig)
	if err != nil {
		return false
	}
	return hmac.Equal(mac.Sum(nil), got)
}
