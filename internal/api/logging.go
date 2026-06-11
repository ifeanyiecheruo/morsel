package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
)

// statusCapture wraps ResponseWriter to capture the status code written by the handler.
// If WriteHeader is never called, status stays at 200 (the implicit default).
type statusCapture struct {
	http.ResponseWriter
	status int
}

func (sc *statusCapture) WriteHeader(status int) {
	sc.status = status
	sc.ResponseWriter.WriteHeader(status)
}

// injectLogger returns middleware that stores logger in every request's context
// so all downstream handlers can call ctxlog.From(req.Context()).
func injectLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		next.ServeHTTP(resp, req.WithContext(ctxlog.With(req.Context(), logger)))
	})
}

// logRequests reads the logger from the request context and logs method, path,
// status, and latency after the handler returns.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		start := time.Now()
		capture := &statusCapture{ResponseWriter: resp, status: http.StatusOK}
		next.ServeHTTP(capture, req)
		ctxlog.From(req.Context()).Info("request",
			"method", req.Method,
			"path", req.URL.Path,
			"status", capture.status,
			"latency", time.Since(start),
		)
	})
}
