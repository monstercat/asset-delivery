package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	. "github.com/monstercat/asset-delivery"
)

// mockFS implements FileSystem for /sign tests. Info is keyed by
// ObjectKey; ObjectURL returns a fixed string.
type mockFS struct {
	files     map[string]FileInfo
	objectURL string
}

func (m *mockFS) FromVolume(string) FileSystem { return m }
func (m *mockFS) ObjectURL(string) string      { return m.objectURL }
func (m *mockFS) Info(f string) (FileInfo, error) {
	if fi, ok := m.files[f]; ok {
		return fi, nil
	}
	return nil, ErrNoFile
}
func (m *mockFS) ReadCloser(string) (io.ReadCloser, error) {
	return nil, ErrNoFile
}
func (m *mockFS) Write(string, io.Reader, FileInfoWrite) error { return nil }
func (m *mockFS) Delete(string) error                          { return nil }

type mockFileInfo struct {
	cacheControl string
	created      time.Time
}

func (m mockFileInfo) CacheControl() string { return m.cacheControl }
func (m mockFileInfo) Created() time.Time   { return m.created }

func mintSig(t *testing.T, secret []byte, cacheKey string, width uint,
	enc string, expires int64,
) string {
	t.Helper()
	msg := cacheKey + ":" + strconv.FormatUint(uint64(width), 10) +
		":" + enc + ":" + strconv.FormatInt(expires, 10)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil))
}

// objectKeyFor mirrors what signHandler computes, so the test can
// pre-populate the mock FS at the exact key.
func objectKeyFor(cacheKey string, width uint, enc string) string {
	opts := ResizeOptions{
		CacheKey: cacheKey,
		Width:    width,
		Encoding: enc,
		Prefix:   "resized",
	}
	opts.PopulateHash()
	return opts.ObjectKey()
}

const (
	testSecret   = "test-shared-secret"
	testCacheKey = "gold://file/abc"
	testWidth    = uint(512)
	testEnc      = "webp"
)

func newSignServer(fs FileSystem) *Server {
	return &Server{FS: fs, Prefix: "resized"}
}

func signRequest(cacheKey string, width uint, enc, sig string,
	expires int64,
) *http.Request {
	q := "?width=" + strconv.FormatUint(uint64(width), 10) +
		"&encoding=" + enc +
		"&signature=" + sig +
		"&expires=" + strconv.FormatInt(expires, 10)
	return httptest.NewRequest(http.MethodGet, "/sign/"+cacheKey+q, nil)
}

func TestSign_ReturnsUrlWhenWarm(t *testing.T) {
	key := objectKeyFor(testCacheKey, testWidth, testEnc)
	fs := &mockFS{
		files:     map[string]FileInfo{key: mockFileInfo{"", time.Now()}},
		objectURL: "https://gcs/signed",
	}
	s := newSignServer(fs)
	t.Setenv(SignSecretEnv, testSecret)
	expires := time.Now().Add(time.Minute).Unix()
	sig := mintSig(t, []byte(testSecret), testCacheKey, testWidth, testEnc, expires)

	rec := httptest.NewRecorder()
	s.signHandler(rec, signRequest(testCacheKey, testWidth, testEnc, sig, expires))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["url"] != "https://gcs/signed" {
		t.Fatalf("got url %q", body["url"])
	}
}

func TestSign_404WhenMissing(t *testing.T) {
	fs := &mockFS{files: map[string]FileInfo{}, objectURL: "x"}
	s := newSignServer(fs)
	t.Setenv(SignSecretEnv, testSecret)
	expires := time.Now().Add(time.Minute).Unix()
	sig := mintSig(t, []byte(testSecret), testCacheKey, testWidth, testEnc, expires)

	rec := httptest.NewRecorder()
	s.signHandler(rec, signRequest(testCacheKey, testWidth, testEnc, sig, expires))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing object, got %d", rec.Code)
	}
}

func TestSign_404WhenObjectExpired(t *testing.T) {
	key := objectKeyFor(testCacheKey, testWidth, testEnc)
	fs := &mockFS{
		files: map[string]FileInfo{
			key: mockFileInfo{"max-age=1", time.Now().Add(-2 * time.Second)},
		},
		objectURL: "x",
	}
	s := newSignServer(fs)
	t.Setenv(SignSecretEnv, testSecret)
	expires := time.Now().Add(time.Minute).Unix()
	sig := mintSig(t, []byte(testSecret), testCacheKey, testWidth, testEnc, expires)

	rec := httptest.NewRecorder()
	s.signHandler(rec, signRequest(testCacheKey, testWidth, testEnc, sig, expires))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for expired object, got %d", rec.Code)
	}
}

func TestSign_404WhenSignatureInvalid(t *testing.T) {
	fs := &mockFS{
		files:     map[string]FileInfo{"x": mockFileInfo{"", time.Now()}},
		objectURL: "x",
	}
	s := newSignServer(fs)
	t.Setenv(SignSecretEnv, testSecret)
	expires := time.Now().Add(time.Minute).Unix()

	rec := httptest.NewRecorder()
	s.signHandler(rec, signRequest(testCacheKey, testWidth, testEnc, "deadbeef", expires))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for bad sig, got %d", rec.Code)
	}
}

func TestSign_404WhenExpiresPast(t *testing.T) {
	key := objectKeyFor(testCacheKey, testWidth, testEnc)
	fs := &mockFS{
		files:     map[string]FileInfo{key: mockFileInfo{"", time.Now()}},
		objectURL: "x",
	}
	s := newSignServer(fs)
	t.Setenv(SignSecretEnv, testSecret)
	expires := time.Now().Add(-time.Minute).Unix() // past
	sig := mintSig(t, []byte(testSecret), testCacheKey, testWidth, testEnc, expires)

	rec := httptest.NewRecorder()
	s.signHandler(rec, signRequest(testCacheKey, testWidth, testEnc, sig, expires))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for expired request, got %d", rec.Code)
	}
}

func TestSign_404WhenNoSecret(t *testing.T) {
	key := objectKeyFor(testCacheKey, testWidth, testEnc)
	fs := &mockFS{
		files:     map[string]FileInfo{key: mockFileInfo{"", time.Now()}},
		objectURL: "x",
	}
	s := newSignServer(fs)
	t.Setenv(SignSecretEnv, "") // no secret configured
	expires := time.Now().Add(time.Minute).Unix()
	sig := mintSig(t, []byte("irrelevant"), testCacheKey, testWidth, testEnc, expires)

	rec := httptest.NewRecorder()
	s.signHandler(rec, signRequest(testCacheKey, testWidth, testEnc, sig, expires))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 with no secret, got %d", rec.Code)
	}
}

func TestVerifySignSig_TamperedWidth(t *testing.T) {
	secret := []byte(testSecret)
	expires := time.Now().Add(time.Minute).Unix()
	sig := mintSig(t, secret, testCacheKey, testWidth, testEnc, expires)
	// sig minted for width 512; verify against a different width.
	if verifySignSig(secret, testCacheKey, 256, testEnc, sig, expires) {
		t.Fatal("expected sig to be invalid for tampered width")
	}
}

func TestVerifySignSig_Valid(t *testing.T) {
	secret := []byte(testSecret)
	expires := time.Now().Add(time.Minute).Unix()
	sig := mintSig(t, secret, testCacheKey, testWidth, testEnc, expires)
	if !verifySignSig(secret, testCacheKey, testWidth, testEnc, sig, expires) {
		t.Fatal("expected sig to be valid")
	}
}
