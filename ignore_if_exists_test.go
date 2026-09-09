package asset_delivery

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// memFS is an in-memory FileSystem for Resize tests. Info reads from
// files; Write records each ObjectKey passed to it.
type memFS struct {
	files   map[string]FileInfo
	written []string
}

func (m *memFS) FromVolume(string) FileSystem { return m }
func (m *memFS) ObjectURL(string) string      { return "mem://object" }
func (m *memFS) Info(f string) (FileInfo, error) {
	if fi, ok := m.files[f]; ok {
		return fi, nil
	}
	return nil, ErrNoFile
}
func (m *memFS) ReadCloser(string) (io.ReadCloser, error) { return nil, ErrNoFile }
func (m *memFS) Write(f string, _ io.Reader, _ FileInfoWrite) error {
	m.written = append(m.written, f)
	return nil
}
func (m *memFS) Delete(string) error { return nil }

type memFileInfo struct {
	cc      string
	created time.Time
}

func (m memFileInfo) CacheControl() string { return m.cc }
func (m memFileInfo) Created() time.Time   { return m.created }

// pngServer serves the 1x1 PNG fixture from resize_test.go so the
// "proceeds" paths can exercise GetImage without live GCS.
func pngServer(t *testing.T) *httptest.Server {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(PngFileB64)
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(b)
	}))
}

func resizeOpts(t *testing.T, location string, ignoreIfExists bool) ResizeOptions {
	t.Helper()
	opts := ResizeOptions{
		Location:       location,
		Width:          512,
		Prefix:         "resized",
		IgnoreIfExists: ignoreIfExists,
	}
	opts.PopulateHash()
	return opts
}

func TestResize_IgnoreIfExists_SkipsWhenFresh(t *testing.T) {
	opts := resizeOpts(t, "https://example.com/src.png", true)
	fs := &memFS{
		files: map[string]FileInfo{
			opts.ObjectKey(): memFileInfo{"", time.Now()},
		},
	}
	if err := Resize(fs, opts); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if len(fs.written) != 0 {
		t.Fatalf("fresh object should not be rewritten; got %d writes: %v", len(fs.written), fs.written)
	}
}

func TestResize_IgnoreIfExists_FillsWhenMissing(t *testing.T) {
	srv := pngServer(t)
	defer srv.Close()
	opts := resizeOpts(t, srv.URL, true)
	fs := &memFS{files: map[string]FileInfo{}}
	if err := Resize(fs, opts); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if len(fs.written) != 1 || fs.written[0] != opts.ObjectKey() {
		t.Fatalf("missing object should be filled; got writes %v", fs.written)
	}
}

func TestResize_IgnoreIfExists_RefreshesWhenStale(t *testing.T) {
	srv := pngServer(t)
	defer srv.Close()
	opts := resizeOpts(t, srv.URL, true)
	fs := &memFS{
		files: map[string]FileInfo{
			// max-age=1, created 2s ago -> IsExpired true
			opts.ObjectKey(): memFileInfo{"max-age=1", time.Now().Add(-2 * time.Second)},
		},
	}
	if err := Resize(fs, opts); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if len(fs.written) != 1 || fs.written[0] != opts.ObjectKey() {
		t.Fatalf("stale object should be refreshed; got writes %v", fs.written)
	}
}

func TestResize_NoIgnore_AlwaysWrites(t *testing.T) {
	srv := pngServer(t)
	defer srv.Close()
	opts := resizeOpts(t, srv.URL, false)
	fs := &memFS{
		files: map[string]FileInfo{
			opts.ObjectKey(): memFileInfo{"max-age=9999", time.Now()},
		},
	}
	if err := Resize(fs, opts); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if len(fs.written) != 1 {
		t.Fatalf("IgnoreIfExists=false must always write; got %d writes", len(fs.written))
	}
}

func TestIsExpired(t *testing.T) {
	cases := []struct {
		name string
		cc   string
		age  time.Duration
		want bool
	}{
		{"fresh within max-age", "max-age=3600", 0, false},
		{"stale past max-age", "max-age=1", 2 * time.Second, true},
		{"no max-age never expires", "", 24 * time.Hour, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fi := memFileInfo{c.cc, time.Now().Add(-c.age)}
			if got := IsExpired(fi); got != c.want {
				t.Fatalf("IsExpired(cc=%q, age=%v) = %v, want %v", c.cc, c.age, got, c.want)
			}
		})
	}
}
