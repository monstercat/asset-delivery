package asset_delivery

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const (
	PngFileB64  = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	WebPFileB64 = "UklGRlYAAABXRUJQVlA4WAoAAAAQAAAAAAAAAAAAQUxQSAIAAAAAf1ZQOCAuAAAA0AEAnQEqAQABAAFAJiWgAnS6AfgAA7AA/vPfZ/5sCBmh9MH/ppHjSPGkfKaAAA=="
	JpegFileB64 = "/9j/2wCEAAYEBQYFBAYGBQYHBwYIChAKCgkJChQODwwQFxQYGBcUFhYaHSUfGhsjHBYWICwgIyYnKSopGR8tMC0oMCUoKSgBBwcHCggKEwoKEygaFhooKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKCgoKP/AABEIAAEAAQMBIgACEQEDEQH/xAGiAAABBQEBAQEBAQAAAAAAAAAAAQIDBAUGBwgJCgsQAAIBAwMCBAMFBQQEAAABfQECAwAEEQUSITFBBhNRYQcicRQygZGhCCNCscEVUtHwJDNicoIJChYXGBkaJSYnKCkqNDU2Nzg5OkNERUZHSElKU1RVVldYWVpjZGVmZ2hpanN0dXZ3eHl6g4SFhoeIiYqSk5SVlpeYmZqio6Slpqeoqaqys7S1tre4ubrCw8TFxsfIycrS09TV1tfY2drh4uPk5ebn6Onq8fLz9PX29/j5+gEAAwEBAQEBAQEBAQAAAAAAAAECAwQFBgcICQoLEQACAQIEBAMEBwUEBAABAncAAQIDEQQFITEGEkFRB2FxEyIygQgUQpGhscEJIzNS8BVictEKFiQ04SXxFxgZGiYnKCkqNTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqCg4SFhoeIiYqSk5SVlpeYmZqio6Slpqeoqaqys7S1tre4ubrCw8TFxsfIycrS09TV1tfY2dri4+Tl5ufo6ery8/T19vf4+fr/2gAMAwEAAhEDEQA/APOKKKK+PPgj/9k="
)

func TestReaderToImage(t *testing.T) {
	cases := []struct {
		Name           string
		Base64         string
		Hint           string
		ExpectedFormat string
	}{
		{"PNG with .png hint", PngFileB64, "a.png", "png"},
		{"PNG with mismatched .webp hint", PngFileB64, "a.webp", "png"},
		{"PNG with mismatched .jpeg hint", PngFileB64, "a.jpeg", "png"},
		{"WebP with .webp hint", WebPFileB64, "a.webp", "webp"},
		{"WebP with mismatched .png hint", WebPFileB64, "a.png", "webp"},
		{"JPEG with .jpeg hint", JpegFileB64, "a.jpeg", "jpeg"},
		// Regression: extensionless URLs (e.g., /api/artist/{uuid}/cover)
		// must still decode and report the detected format so the caller
		// can pick an output encoding.
		{"PNG with extensionless hint", PngFileB64, "/api/artist/uuid/cover", "png"},
		{"WebP with extensionless hint", WebPFileB64, "/api/artist/uuid/cover", "webp"},
		{"JPEG with extensionless hint", JpegFileB64, "/api/artist/uuid/cover", "jpeg"},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			b, err := base64.StdEncoding.DecodeString(c.Base64)
			if err != nil {
				t.Fatal(err)
			}

			img, format, err := ReaderToImage(bytes.NewReader(b), c.Hint)
			if err != nil {
				t.Fatal(err)
			}
			if img == nil {
				t.Fatal("expected decoded image, got nil")
			}
			if format != c.ExpectedFormat {
				t.Fatalf("expected detected format %q, got %q", c.ExpectedFormat, format)
			}
		})
	}
}

func TestResolveEncoding(t *testing.T) {
	cases := []struct {
		Name     string
		Hint     string
		Detected string
		Want     string
	}{
		{"explicit webp hint wins", ".webp", "jpeg", ".webp"},
		{"jpg hint normalized through case", ".JPG", "png", ".JPG"},
		{"empty hint falls back to detected", "", "png", ".png"},
		{"unknown hint falls back to detected", ".tiff", "webp", ".webp"},
		{"both empty stays empty (caller surfaces ErrFileNotHandled)", "", "", ""},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			got := resolveEncoding(c.Hint, c.Detected)
			if got != c.Want {
				t.Fatalf("resolveEncoding(%q, %q) = %q; want %q", c.Hint, c.Detected, got, c.Want)
			}
		})
	}
}

func TestResolveFetchTimeout(t *testing.T) {
	cases := []struct {
		Name string
		Raw  string
		Want time.Duration
	}{
		{"unset falls back", "", 60 * time.Second},
		{"valid duration wins", "90s", 90 * time.Second},
		{"minutes parse too", "2m", 2 * time.Minute},
		{"surrounding space tolerated", "  45s  ", 45 * time.Second},
		{"unparseable falls back", "not-a-duration", 60 * time.Second},
		// A bare number is not a valid Go duration. Falling back matters here:
		// silently reading "30" as nanoseconds would make every fetch fail.
		{"bare number falls back", "30", 60 * time.Second},
		// Zero and negative must not disable the timeout — an unbounded fetch
		// pins a worker for as long as the origin holds the connection.
		{"zero falls back", "0s", 60 * time.Second},
		{"negative falls back", "-5s", 60 * time.Second},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			got := resolveFetchTimeout(c.Raw, 60*time.Second)
			if got != c.Want {
				t.Fatalf("resolveFetchTimeout(%q) = %s; want %s", c.Raw, got, c.Want)
			}
		})
	}
}

// TestGetImageErrorClassification pins the retry contract GetImage owes the
// worker: the returned status is what decides whether Pub/Sub retries the
// message (5xx) or dead-letters it (4xx). Misclassifying a transient failure as
// permanent is what left variants ungenerated forever.
func TestGetImageErrorClassification(t *testing.T) {
	cases := []struct {
		Name       string
		Status     int
		WantStatus int
	}{
		{"upstream 500 is retryable", http.StatusInternalServerError, http.StatusInternalServerError},
		{"upstream 502 is retryable", http.StatusBadGateway, http.StatusInternalServerError},
		{"upstream 503 is retryable", http.StatusServiceUnavailable, http.StatusInternalServerError},
		{"rate limiting is retryable", http.StatusTooManyRequests, http.StatusInternalServerError},
		{"upstream timeout is retryable", http.StatusRequestTimeout, http.StatusInternalServerError},
		{"missing source is permanent", http.StatusNotFound, http.StatusBadRequest},
		{"forbidden source is permanent", http.StatusForbidden, http.StatusBadRequest},
		{"gone source is permanent", http.StatusGone, http.StatusBadRequest},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(c.Status)
			}))
			defer srv.Close()

			_, _, err := GetImage(srv.URL)
			if err == nil {
				t.Fatal("expected an error for a non-2xx upstream, got nil")
			}
			httpErr, ok := err.(HTTPError)
			if !ok {
				t.Fatalf("expected an HTTPError so the worker can map it to an ack decision, got %T", err)
			}
			if httpErr.Status() != c.WantStatus {
				t.Fatalf("upstream %d classified as %d; want %d", c.Status, httpErr.Status(), c.WantStatus)
			}
		})
	}
}

// TestGetImageTimeoutIsRetryable is the original bug in miniature: a fetch that
// runs past the budget must be reported as retryable, not as a bad parameter.
func TestGetImageTimeoutIsRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	restore := imageFetchTimeout
	imageFetchTimeout = 20 * time.Millisecond
	defer func() { imageFetchTimeout = restore }()

	_, _, err := GetImage(srv.URL)
	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	httpErr, ok := err.(HTTPError)
	if !ok {
		t.Fatalf("expected an HTTPError, got %T", err)
	}
	if httpErr.Status() != http.StatusInternalServerError {
		t.Fatalf("timeout classified as %d; want 500 so Pub/Sub retries it", httpErr.Status())
	}
}

// TestGetImageSucceedsAndReportsCacheControl covers the happy path, including
// that the upstream Cache-Control is handed back for Resize to persist.
func TestGetImageSucceedsAndReportsCacheControl(t *testing.T) {
	body, err := base64.StdEncoding.DecodeString(JpegFileB64)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "max-age=1234")
		w.Write(body)
	}))
	defer srv.Close()

	buf, cc, err := GetImage(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf, body) {
		t.Fatalf("expected the upstream bytes back, got %d of %d", len(buf), len(body))
	}
	if cc != "max-age=1234" {
		t.Fatalf("expected the upstream Cache-Control, got %q", cc)
	}
}

// TestResizePreservesFetchClassification guards the seam between GetImage and
// Resize. Resize used to re-wrap every fetch error as a ParamError, collapsing
// a retryable failure into a permanent one; the classification must survive.
//
// A nil FileSystem is fine: the fetch fails before Resize touches storage.
func TestResizePreservesFetchClassification(t *testing.T) {
	cases := []struct {
		Name       string
		Status     int
		WantStatus int
	}{
		{"retryable upstream failure stays 5xx", http.StatusServiceUnavailable, http.StatusInternalServerError},
		{"permanent upstream failure stays 4xx", http.StatusNotFound, http.StatusBadRequest},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(c.Status)
			}))
			defer srv.Close()

			err := Resize(nil, ResizeOptions{Location: srv.URL, Width: 512})
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			httpErr, ok := err.(HTTPError)
			if !ok {
				t.Fatalf("expected an HTTPError, got %T", err)
			}
			if httpErr.Status() != c.WantStatus {
				t.Fatalf("Resize reported %d for upstream %d; want %d", httpErr.Status(), c.Status, c.WantStatus)
			}
		})
	}
}
