package asset_delivery

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/chai2010/webp"
	"github.com/disintegration/imaging"
)

// PubSub topic for sending and receiving resize request
const ResizeTopic = "asset-delivery-resize"

func Resize(fs FileSystem, opts ResizeOptions) error {
	buf, cc, err := GetImage(opts.Location)
	if err != nil {
		return &ParamError{Param: "url", Detail: fmt.Sprintf("Could not get image: %s", opts.Location), RootError: err}
	}
	img, format, err := ReaderToImage(bytes.NewReader(buf), opts.Location)
	if err != nil {
		return &ParamError{Param: "url", Detail: "Could not read URL as an image.", RootError: err}
	}
	img, err = ResizeImage(img, opts.Width)
	if err != nil {
		return &SystemError{Detail: "Could not resize the provided image.", RootError: err}
	}
	bits, err := ImageToBytes(img, resolveEncoding(opts.DesiredEncoding(), format), 80)
	if err != nil {
		return &SystemError{Detail: "An error occurred.", RootError: err}
	}
	if cc == "" {
		if opts.CacheControl == "" {
			cc = defaultCacheControl
		} else {
			cc = opts.CacheControl
		}
	}
	if err := fs.Write(opts.ObjectKey(), bits, &WriteInfo{cacheControl: cc}); err != nil {
		return &SystemError{Detail: "An error occurred.", RootError: err}
	}
	return nil
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

func ResizeImage(img image.Image, target uint) (image.Image, error) {
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

func DefaultImageDecode(r io.Reader) (image.Image, error) {
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

// ReaderToImage decodes r as an image, using hint's extension to pick
// the decoder when it names a supported format and falling back to
// image.Decode otherwise. The returned format is the auto-detected
// format name ("jpeg", "png", "webp", ...) when the fallback path is
// taken, or the format implied by the hint when the typed decoder
// succeeds. Callers can use it to choose an output encoding when the
// hint URL has no extension.
func ReaderToImage(r io.ReadSeeker, hint string) (image.Image, string, error) {
	ext := strings.ToLower(filepath.Ext(hint))
	var fn func(io.Reader) (image.Image, error)
	var formatHint string
	switch ext {
	case ".jpeg", ".jfif", ".jpg":
		fn = jpeg.Decode
		formatHint = "jpeg"
	case ".png":
		fn = png.Decode
		formatHint = "png"
	case ".webp":
		fn = webp.Decode
		formatHint = "webp"
	}

	if fn != nil {
		if img, err := fn(r); err == nil {
			return img, formatHint, nil
		}
		if _, err := r.Seek(0, io.SeekStart); err != nil {
			return nil, "", err
		}
	}

	img, format, err := image.Decode(r)
	if err != nil {
		return nil, "", &ParamError{
			Param:     "url",
			RootError: err,
			Detail:    "Unsupported image format.",
		}
	}
	return img, format, nil
}

// resolveEncoding picks the output extension for ImageToBytes. It
// prefers the caller-supplied hint when it names a supported format
// (covering the explicit `encoding=` query param and URLs with usable
// extensions); otherwise it falls back to the format detected during
// decode. Both inputs being empty/unknown returns the hint unchanged,
// which lets ImageToBytes surface ErrFileNotHandled as before.
func resolveEncoding(hint, detected string) string {
	switch strings.ToLower(hint) {
	case ".jpeg", ".jfif", ".jpg", ".png", ".webp":
		return hint
	}
	if detected != "" {
		return "." + detected
	}
	return hint
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
