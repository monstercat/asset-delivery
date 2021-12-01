package main

import (
	"bytes"
	"crypto/sha1"
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
)

type Server struct {
	FS FileSystem
}

type ResizeOptions struct {
	Width uint64
	Location string
	HashSum  string
	Encoding string
	Prefix   string
}

func (opts *ResizeOptions) ObjectKey() string {
	return fmt.Sprintf("%s/%s/%d%s", opts.Prefix, opts.HashSum, opts.Width, opts.DesiredEncoding())
}

func (opts *ResizeOptions) DesiredEncoding() string {
	if len(opts.Encoding) > 0 {
		return "." + opts.Encoding
	}
	return filepath.Ext(opts.Location)
}

func NewResizeOptionsFromQuery(m map[string][]string) (ResizeOptions, error) {
	var opts ResizeOptions
	if xs, ok := m["width"]; ok {
		var err error
		opts.Width, err = parseUint(xs[0])
		if err != nil {
			return opts, &ParamError{Param: "width", Detail: "Invalid value."}
		}
		if opts.Width <= 0 || opts.Width > MaxImageDimension {
			return opts, &ParamError{Param: "width", Detail: "Expected a width greater than 0 and less than 4096."}
		}
	}
	if xs, ok := m["url"]; ok {
		opts.Location = strings.TrimSpace(xs[0])
	}
	if opts.Location == "" {
		return opts, &ParamError{Param: "url", Detail: "Invalid (or missing) URL."}
	} else {
		hash := sha1.New()
		hash.Write([]byte(opts.Location))
		sum := hash.Sum(nil)
		opts.HashSum = fmt.Sprintf("%x", sum)
		// TODO validate location param, we'll just let HTTP request validate for now
	}

	if xs, ok := m["encoding"]; ok {
		opts.Encoding = strings.TrimSpace(xs[0])
		// TODO validate encoding param, we'll let image encoder default for now
	}
	return opts, nil
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
	if err := s.Resize(opts); err != nil {
		WriteError(w, err)
		return
	}
	http.Redirect(w, r, s.FS.ObjectURL(opts.ObjectKey()), http.StatusPermanentRedirect)
}

func (s *Server) Resize(opts ResizeOptions) error {
	ok, err := s.FS.Exists(opts.ObjectKey())
	if err != nil {
		return &SystemError{RootError: err, Detail: "Could not check if image already exists."}
	} else if ok {
		return nil
	}
	buf, err := GetImage(opts.Location)
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
	if err := s.FS.Write(opts.ObjectKey(), bits); err != nil {
		return &SystemError{Detail: "An error occurred.", RootError: err}
	}
	return nil
}

func GetImage(url string) ([]byte, error){
	client := http.Client{
		Timeout: time.Second * 5,
	}
	res, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	return io.ReadAll(res.Body)
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
	}
	if fn == nil {
		return nil, ErrFileNotHandled
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
