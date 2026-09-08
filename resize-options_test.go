package asset_delivery

import (
	"crypto/sha1"
	"fmt"
	"testing"
)

func sha1Hex(s string) string {
	return fmt.Sprintf("%x", sha1.Sum([]byte(s)))
}

func TestPopulateHash_UsesCacheKeyWhenPresent(t *testing.T) {
	opts := ResizeOptions{
		CacheKey: "gold://file/abc",
		Location: "https://example.com/src.png",
	}
	opts.PopulateHash()
	want := sha1Hex("gold://file/abc")
	if opts.HashSum != want {
		t.Fatalf("got %s, want %s (CacheKey)", opts.HashSum, want)
	}
	if opts.HashSum == sha1Hex(opts.Location) {
		t.Fatal("hashed Location instead of CacheKey")
	}
}

func TestPopulateHash_FallsBackToLocationLegacy(t *testing.T) {
	opts := ResizeOptions{Location: "https://example.com/src.png"}
	opts.PopulateHash()
	want := sha1Hex("https://example.com/src.png")
	if opts.HashSum != want {
		t.Fatalf("got %s, want %s (Location)", opts.HashSum, want)
	}
}

func TestObjectKey_CacheKeyPath(t *testing.T) {
	opts := ResizeOptions{
		CacheKey: "gold://file/abc",
		Width:    512,
		Encoding: "webp",
		Prefix:   "resized",
	}
	opts.PopulateHash()
	want := "resized/" + sha1Hex("gold://file/abc") + "/512.webp"
	if got := opts.ObjectKey(); got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}
