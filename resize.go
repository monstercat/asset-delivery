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
