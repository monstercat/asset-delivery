package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/marcw/cachecontrol"

	. "github.com/monstercat/asset-delivery"
)

type Server struct {
	FS             FileSystem
	PB             *pubsub.Client
	PermittedHosts []string
	Prefix         string
}

func (s *Server) HostPermitted(host string) bool {
	if len(s.PermittedHosts) == 0 {
		return true
	}
	for _, x := range s.PermittedHosts {
		if strings.TrimSpace(x) == host {
			return true
		}
	}
	return false
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	opts, err := NewResizeOptionsFromQuery(r.URL.Query())
	if err != nil {
		WriteError(w, err)
		return
	}
	opts.Prefix = s.Prefix

	if !s.HostPermitted(opts.URL.Host) {
		WriteError(w, &ParamError{Param: "url", Detail: "Host is not permitted to perform this action."})
		return
	}
	need, err := s.NeedsResizing(opts)
	if err != nil {
		WriteError(w, err)
		return
	}
	if !need {
		http.Redirect(w, r, s.FS.ObjectURL(opts.ObjectKey()), http.StatusPermanentRedirect)
		return
	}

	// Send resize request
	s.sendResize(opts.ResizeOptions)

	http.Redirect(w, r, opts.Location, http.StatusTemporaryRedirect)
}

func (s *Server) sendResize(opts ResizeOptions) {
	// Send resize request
	b, err := json.Marshal(opts)
	if err != nil {
		log.Printf("Could not marshal resize options. %s", err)
		return
	}

	// Publish Immediately.
	topic := s.PB.Topic(ResizeTopic)
	topic.PublishSettings.DelayThreshold = 0
	topic.PublishSettings.CountThreshold = 0

	topic.Publish(context.Background(), &pubsub.Message{
		Data: b,
	})
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
