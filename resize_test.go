package asset_delivery

import (
	"bytes"
	"encoding/base64"
	"testing"
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
