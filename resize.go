package asset_delivery

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chai2010/webp"
	"github.com/disintegration/imaging"
)

// PubSub topic for sending and receiving resize request
const ResizeTopic = "asset-delivery-resize"

// defaultCacheControl is the Cache-Control header value written on
// resized objects when neither the upstream response nor the request
// supplied one.
var defaultCacheControl = os.Getenv("DEFAULT_CACHE_CONTROL")

// defaultImageFetchTimeout bounds the upstream fetch in GetImage.
//
// Sources span three orders of magnitude: a Spotify artist image is 640x640 at
// well under 250 KB, while an unresized cover straight out of Label Manager can
// be 6000x3989 at 34 MB. The budget also has to cover the origin's own redirect
// hop and object-signing latency on top of the transfer itself, so the previous
// 5s left no headroom for the large end of that range.
//
// The budget alone was never the whole story — see GetImage for the reason a
// failure here used to be permanent — but it is the part that made failure
// likely in the first place.
const defaultImageFetchTimeout = 60 * time.Second

// imageFetchTimeout is the active budget, overridable with IMAGE_FETCH_TIMEOUT
// (any time.ParseDuration value) so it can be tuned without shipping code.
//
// Keep the Pub/Sub subscription's ack deadline at or above this value. If the
// deadline is shorter, a slow fetch is redelivered while the first attempt is
// still running and the work is duplicated rather than retried.
var imageFetchTimeout = resolveFetchTimeout(os.Getenv("IMAGE_FETCH_TIMEOUT"), defaultImageFetchTimeout)

// resolveFetchTimeout parses an IMAGE_FETCH_TIMEOUT override, falling back to
// fallback when unset, unparseable or non-positive. A bad value must not be
// able to disable the timeout altogether — an unbounded fetch would pin a
// worker instance for as long as the origin keeps the connection open.
func resolveFetchTimeout(raw string, fallback time.Duration) time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

func Resize(fs FileSystem, opts ResizeOptions) error {
	buf, cc, err := GetImage(opts.Location)
	if err != nil {
		// Passed through rather than re-wrapped: GetImage has already decided
		// whether this failure is worth retrying, and flattening it back into a
		// ParamError here is precisely what made every transient fetch failure
		// permanent.
		return err
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

// GetImage fetches the source image, returning its bytes and the upstream
// Cache-Control header.
//
// Errors are classified by whether a later attempt could plausibly succeed,
// because the worker converts the status directly into a Pub/Sub ack decision
// (see cmd/resize.Server.ServeHTTP): a 4xx dead-letters the message, a 5xx
// retries it. Transport failures, timeouts and upstream 5xx/429/408 are
// therefore SystemError (500, retried), while a definitive upstream rejection
// such as 404 or 403 is a ParamError (400, dead-lettered) — retrying cannot
// bring back a source that is gone.
//
// Getting that split wrong is expensive in one direction: a transient failure
// classified as permanent means the variant is never generated and every
// subsequent delivery request serves the unresized original forever.
func GetImage(url string) ([]byte, string, error) {
	client := http.Client{
		Timeout: imageFetchTimeout,
	}
	res, err := client.Get(url)
	if err != nil {
		// DNS, connection reset, TLS, or the timeout above. None of these say
		// anything about whether the source is valid.
		return nil, "", &SystemError{
			RootError: err,
			Detail:    fmt.Sprintf("Could not fetch image: %s", url),
		}
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, "", statusError(url, res.StatusCode)
	}
	buf, err := io.ReadAll(res.Body)
	if err != nil {
		// A read that dies partway through is a transport failure like any
		// other; the bytes may well arrive in full on the next attempt.
		return nil, "", &SystemError{
			RootError: err,
			Detail:    fmt.Sprintf("Could not read image: %s", url),
		}
	}
	return buf, res.Header.Get("Cache-Control"), nil
}

// statusError maps a non-2xx upstream response onto the retry classification
// described on GetImage.
func statusError(url string, status int) error {
	detail := fmt.Sprintf("Upstream responded %d for image: %s", status, url)
	root := fmt.Errorf("upstream status %d", status)
	switch {
	case status >= 500,
		status == http.StatusTooManyRequests,
		status == http.StatusRequestTimeout:
		return &SystemError{RootError: root, Detail: detail}
	}
	return &ParamError{Param: "url", Detail: detail, RootError: root}
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
