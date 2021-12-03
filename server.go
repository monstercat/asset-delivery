package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chai2010/webp"
	"github.com/disintegration/imaging"
	"github.com/marcw/cachecontrol"
)

const MaxImageDimension = 4096

var ErrFileNotHandled = errors.New("file type not handled")
var ErrInvalidBounds = errors.New("invalid image bounds")

type Server struct {
	FS FileSystem
	PermittedHosts []string
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
	opts.Prefix = "resized"

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
	go func() {
		err := s.Resize(opts)
		log.Printf("Error resizing requested image [%s]: %s", opts.Location, err.Error())
	}()
	http.Redirect(w, r, opts.Location, http.StatusTemporaryRedirect)
}

func isExpired(info FileInfo) bool {
	control := cachecontrol.Parse(info.CacheControl())
	if control.MaxAge() <= 0 {
		return false
	}
	return time.Now().After(info.Created().Add(control.MaxAge()))
}

func (s *Server) NeedsResizing(opts ResizeOptions) (bool, error) {
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

func (s *Server) Resize(opts ResizeOptions) error {
	buf, cc, err := GetImage(opts.Location)
	if err != nil {
		return &ParamError{Param: "url", Detail: fmt.Sprintf("Could not get image: %s", opts.Location), RootError: err}
	}
	img, err := ReaderToImage(bytes.NewReader(buf), opts.Location)
	if err != nil {
		return &ParamError{Param: "url", Detail: "Could not read URL as an image.", RootError: err}
	}
	img, err = ResizeImage(img, opts.Width)
	if err != nil {
		return &SystemError{Detail: "Could not resize the provided image.", RootError: err}
	}
	bits, err := ImageToBytes(img, opts.DesiredEncoding(), 80)
	if err != nil {
		return &SystemError{Detail: "An error occurred.", RootError: err}
	}
	if err := s.FS.Write(opts.ObjectKey(), bits, &WriteInfo{cacheControl: cc}); err != nil {
		return &SystemError{Detail: "An error occurred.", RootError: err}
	}
	return nil
}

type WriteInfo struct {
	cacheControl string
}

func (i *WriteInfo) CacheControl() string {
	return i.cacheControl
}

func WriteError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if v, ok := err.(HTTPError); ok {
		status = v.Status()
	}
	w.WriteHeader(status)
	w.Write([]byte(err.Error()))

	if v, ok := err.(RootError); ok && v.Root() != nil {
		log.Println(v.Root())
	}
}

func GetImage(url string) ([]byte, string, error) {
	client := http.Client{
		Timeout: time.Second * 5,
	}
	res, err := client.Get(url)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	buf, err := io.ReadAll(res.Body)
	return buf, res.Header.Get("Cache-Control"), err
}

func ResizeImage(img image.Image, target uint64) (image.Image, error) {
	bounds := img.Bounds()
	width := bounds.Max.X
	height := bounds.Max.Y
	if bounds.Max.X <= 0 {
		return nil, ErrInvalidBounds
	}
	ratio := float64(bounds.Max.Y) / float64(bounds.Max.X)
	height = int(float64(target) * ratio)
	width = int(target)
	return imaging.Resize(img, width, height, imaging.Lanczos), nil
}

func ReaderToImage(r io.Reader, hint string) (image.Image, error) {
	ext := strings.ToLower(filepath.Ext(hint))
	var fn func(io.Reader) (image.Image, error)
	switch ext {
	case ".jpeg", ".jfif", ".jpg":
		fn = jpeg.Decode
	case ".png":
		fn = png.Decode
	case ".webp":
		fn = webp.Decode
	default:
		fn = func(r io.Reader) (image.Image, error) {
			img, _, err := image.Decode(r)
			if err != nil {
				return nil, &ParamError{
					Param:     "url",
					RootError: err,
					Detail:    "Unsupported image format.",
				}
			}
			return img, nil
		}
	}
	return fn(r)
}

func ImageToBytes(i image.Image, hint string, quality int) (*bytes.Buffer, error) {
	ext := strings.ToLower(filepath.Ext(hint))
	buf := bytes.NewBuffer([]byte{})
	var err error
	switch ext {
	case ".jpeg", ".jfif", ".jpg":
		err = jpeg.Encode(buf, i, &jpeg.Options{Quality: quality})
	case ".png":
		err = png.Encode(buf, i)
	case ".webp":
		err = webp.Encode(buf, i, &webp.Options{Quality: float32(quality)})
	default:
		err = ErrFileNotHandled
	}
	return buf, err
}

func parseUint(str string) (uint64, error) {
	size, err := strconv.ParseUint(str, 10, 32)
	if err != nil {
		return 0, errors.New("bad image size provided")
	}
	return size, nil
}
