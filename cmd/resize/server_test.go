package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/monstercat/golib/logger"
)

type noopLogger struct{}

func (noopLogger) Log(_ logger.Severity, _ any) {}

func newTestServer() *Server {
	return &Server{Logger: noopLogger{}}
}

func TestServeHTTP_RejectsNonPOST(t *testing.T) {
	s := newTestServer()
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestServeHTTP_RejectsMalformedEnvelope(t *testing.T) {
	s := newTestServer()
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("not json"))))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed envelope, got %d", rec.Code)
	}
}

func TestServeHTTP_RejectsMalformedMessageData(t *testing.T) {
	envelope := map[string]any{
		"message": map[string]any{
			"data":      base64.StdEncoding.EncodeToString([]byte("not json either")),
			"messageId": "test-msg-1",
		},
		"subscription": "projects/test/subscriptions/sub",
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}

	s := newTestServer()
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad message data, got %d", rec.Code)
	}
}
