package api

import (
	"log/slog"
	"net/http"
	"time"
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

func logRequests(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		start := time.Now()
		capture := &statusCapture{ResponseWriter: resp, status: http.StatusOK}
		next.ServeHTTP(capture, req)
		logger.Info("request",
			"method", req.Method,
			"path", req.URL.Path,
			"status", capture.status,
			"latency", time.Since(start),
		)
	})
}
