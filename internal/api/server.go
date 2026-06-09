package api

import (
	"fmt"
	"net/http"

	"github.com/ifeanyiecheruo/morsel/platform"
)

// NewMux constructs the root HTTP mux for the Morsel API.
// The platform is passed here for injection into handlers as they are added.
func NewMux(plat platform.Platform) http.Handler {
	_ = plat // used by handlers added in subsequent features
	mux := http.NewServeMux()

	route := func(pattern string, handler func(http.ResponseWriter, *http.Request) error) {
		mux.Handle(pattern, ErrorHandlerFunc(handler))
	}

	route("GET /healthz", handleHealthz)

	route("/", func(resp http.ResponseWriter, req *http.Request) error {
		return &APIError{
			HTTPStatus: http.StatusNotFound,
			Code:       "not_found",
			Message:    fmt.Sprintf("%s %s not found", req.Method, req.URL.Path),
			Remedy:     "check the API documentation for valid endpoints",
		}
	})

	return mux
}
