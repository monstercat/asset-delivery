package main

import (
	"crypto/sha1"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

type ResizeOptions struct {
	Width    uint64
	Location string
	URL      *url.URL
	HashSum  string
	Encoding string
	Prefix   string
	Force    bool
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
		var err error
		opts.URL, err = url.Parse(opts.Location)
		if err != nil {
			return opts, &ParamError{Param: "url", Detail: "Invalid URL provided.", RootError: err}
		}
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
	if _, ok := m["force"]; ok {
		opts.Force = true
	}

	if xs, ok := m["encoding"]; ok {
		opts.Encoding = strings.TrimSpace(xs[0])
		// TODO validate encoding param, we'll let image encoder default for now
	}
	return opts, nil
}
