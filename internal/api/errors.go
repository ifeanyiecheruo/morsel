package api

import (
	"encoding/json"
	"errors"
	"net/http"
)

// ErrorHandlerFunc is an http.Handler whose ServeHTTP writes a structured JSON
// error if the handler returns a non-nil error.
type ErrorHandlerFunc func(http.ResponseWriter, *http.Request) error

func (fn ErrorHandlerFunc) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	if err := fn(resp, req); err != nil {
		writeError(resp, err)
	}
}

// APIError is the structured error type returned by all Morsel API endpoints.
// Shape per conventions/rest.md: {"error": {"code", "message", "remedy", "context"}}.
type APIError struct {
	HTTPStatus int            `json:"-"`
	Code       string         `json:"code"`
	Message    string         `json:"message"`
	Remedy     string         `json:"remedy"`
	Context    map[string]any `json:"context,omitempty"`
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

func writeError(resp http.ResponseWriter, err error) {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		apiErr = &APIError{
			HTTPStatus: http.StatusInternalServerError,
			Code:       "internal_error",
			Message:    "an unexpected error occurred",
			Remedy:     "contact your platform operator if the issue persists",
		}
	}
	resp.Header().Set("Content-Type", "application/json")
	resp.WriteHeader(apiErr.HTTPStatus)
	json.NewEncoder(resp).Encode(struct { //nolint:errcheck
		Error *APIError `json:"error"`
	}{Error: apiErr})
}
