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
		Base64 string
		Hint   string
	}{
		{PngFileB64, "a.png"},
		{PngFileB64, "a.webp"},
		{PngFileB64, "a.jpeg"},
		{WebPFileB64, "a.webp"},
		{WebPFileB64, "a.png"},
		{JpegFileB64, "a.jpeg"},
	}

	for _, c := range cases {
		b, err := base64.StdEncoding.DecodeString(c.Base64)
		if err != nil {
			t.Fatal(err)
		}

		_, err = ReaderToImage(bytes.NewReader(b), c.Hint)
		if err != nil {
			t.Fatal(err)
		}
	}
}
