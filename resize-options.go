package asset_delivery

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
)

const MaxImageDimension = 4096

type ResizeOptions struct {
	Width    uint64
	Location string
	HashSum  string
	Encoding string
	Prefix   string
}

type ResizeOptionsProcessed struct {
	ResizeOptions
	URL   *url.URL
	Force bool
}

func (opts *ResizeOptions) PopulateHash() {
	hash := sha1.New()
	hash.Write([]byte(opts.Location))
	sum := hash.Sum(nil)
	opts.HashSum = fmt.Sprintf("%x", sum)
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

func NewResizeOptionsFromQuery(m map[string][]string) (ResizeOptionsProcessed, error) {
	var opts ResizeOptionsProcessed
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
		opts.PopulateHash()
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

func parseUint(str string) (uint64, error) {
	size, err := strconv.ParseUint(str, 10, 32)
	if err != nil {
		return 0, errors.New("bad image size provided")
	}
	return size, nil
}

type WriteInfo struct {
	cacheControl string
}

func (i *WriteInfo) CacheControl() string {
	return i.cacheControl
}
