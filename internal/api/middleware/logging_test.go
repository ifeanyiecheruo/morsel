package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type capturedRecord struct {
	message string
	attrs   map[string]any
}

type captureHandler struct {
	mu      sync.Mutex
	records []capturedRecord
}

func (ch *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (ch *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler         { return ch }
func (ch *captureHandler) WithGroup(_ string) slog.Handler              { return ch }
func (ch *captureHandler) Handle(_ context.Context, rec slog.Record) error {
	attrs := make(map[string]any)
	rec.Attrs(func(attr slog.Attr) bool {
		attrs[attr.Key] = attr.Value.Any()
		return true
	})
	ch.mu.Lock()
	ch.records = append(ch.records, capturedRecord{message: rec.Message, attrs: attrs})
	ch.mu.Unlock()
	return nil
}

func newTestLogger() (*slog.Logger, *captureHandler) {
	handler := &captureHandler{}
	return slog.New(handler), handler
}

func TestLogRequestsRecordsFields(t *testing.T) {
	logger, handler := newTestLogger()
	inner := http.HandlerFunc(func(resp http.ResponseWriter, _ *http.Request) {
		resp.WriteHeader(http.StatusCreated)
	})
	mux := InjectLogger(logger, LogRequests(inner))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/things", nil))

	handler.mu.Lock()
	records := handler.records
	handler.mu.Unlock()

	if len(records) != 1 {
		t.Fatalf("got %d log records, want 1", len(records))
	}
	entry := records[0]
	if entry.message != "request" {
		t.Errorf("message = %q, want request", entry.message)
	}
	if entry.attrs["method"] != "POST" {
		t.Errorf("method = %v, want POST", entry.attrs["method"])
	}
	if entry.attrs["path"] != "/api/things" {
		t.Errorf("path = %v, want /api/things", entry.attrs["path"])
	}
	if entry.attrs["status"] != int64(http.StatusCreated) {
		t.Errorf("status = %v (%T), want 201", entry.attrs["status"], entry.attrs["status"])
	}
	if _, ok := entry.attrs["latency"]; !ok {
		t.Error("latency field missing from log record")
	}
}

func TestLogRequestsDefaultsStatusTo200(t *testing.T) {
	logger, handler := newTestLogger()
	inner := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		// write no explicit status — implicit 200
	})
	InjectLogger(logger, LogRequests(inner)).ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/healthz", nil),
	)

	handler.mu.Lock()
	records := handler.records
	handler.mu.Unlock()

	if len(records) != 1 {
		t.Fatalf("got %d log records, want 1", len(records))
	}
	if records[0].attrs["status"] != int64(http.StatusOK) {
		t.Errorf("status = %v, want 200", records[0].attrs["status"])
	}
}
