package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIErrorFormat(t *testing.T) {
	err := &APIError{Code: "quota_exceeded", Message: "too many apps"}
	if got := err.Error(); got != "quota_exceeded: too many apps" {
		t.Errorf("Error() = %q, want %q", got, "quota_exceeded: too many apps")
	}
}

func TestWriteErrorShape(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, &APIError{
		HTTPStatus: http.StatusConflict,
		Code:       "quota_exceeded",
		Message:    "limit reached",
		Remedy:     "contact operator",
		Context:    map[string]any{"current": 2, "limit": 2},
	})

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
	if contentType := rec.Header().Get("Content-Type"); contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}

	var body struct {
		Error struct {
			Code    string         `json:"code"`
			Message string         `json:"message"`
			Remedy  string         `json:"remedy"`
			Context map[string]any `json:"context"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != "quota_exceeded" {
		t.Errorf("code = %q, want quota_exceeded", body.Error.Code)
	}
	if body.Error.Remedy != "contact operator" {
		t.Errorf("remedy = %q, want %q", body.Error.Remedy, "contact operator")
	}
	if body.Error.Context == nil {
		t.Error("context is nil, want populated")
	}
}

func TestWriteErrorUnknownBecomesInternalError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, errors.New("something unexpected"))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	json.NewDecoder(rec.Body).Decode(&body) //nolint:errcheck
	if body.Error.Code != "internal_error" {
		t.Errorf("code = %q, want internal_error", body.Error.Code)
	}
}

func TestErrorHandlerFuncNilReturnWritesNothing(t *testing.T) {
	handler := ErrorHandlerFunc(func(resp http.ResponseWriter, _ *http.Request) error {
		resp.WriteHeader(http.StatusOK)
		return nil
	})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("body = %q, want empty", rec.Body.String())
	}
}

func TestErrorHandlerFuncErrorWritesStructuredResponse(t *testing.T) {
	handler := ErrorHandlerFunc(func(_ http.ResponseWriter, _ *http.Request) error {
		return &APIError{
			HTTPStatus: http.StatusUnauthorized,
			Code:       "invalid_token",
			Message:    "token expired",
			Remedy:     "re-authenticate",
		}
	})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	json.NewDecoder(rec.Body).Decode(&body) //nolint:errcheck
	if body.Error.Code != "invalid_token" {
		t.Errorf("code = %q, want invalid_token", body.Error.Code)
	}
}
