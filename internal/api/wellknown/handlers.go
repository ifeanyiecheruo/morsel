package wellknown

import (
	"encoding/json"
	"net/http"

	openapispec "github.com/ifeanyiecheruo/morsel/api/openapi"
)

// handlers serves the three /.well-known/ discovery documents.
type handlers struct {
	spec []byte
	mux  http.Handler
}

// New bundles the embedded OpenAPI spec and registers routes under basePath
// (e.g. "/.well-known").
func New(basePath string) (http.Handler, error) {
	spec, err := BundleSpec(openapispec.FS, "openapi.yaml")
	if err != nil {
		return nil, err
	}

	h := &handlers{spec: spec}

	mux := http.NewServeMux()
	mux.HandleFunc("GET "+basePath+"/openapi", h.handleOpenAPI)
	mux.HandleFunc("GET "+basePath+"/api-catalog", h.handleCatalog)
	mux.HandleFunc("GET "+basePath+"/ai-plugin.json", h.handleAIPlugin)
	h.mux = mux

	return h, nil
}

func (h *handlers) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *handlers) handleOpenAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/openapi+json")
	_, _ = w.Write(h.spec)
}

func (h *handlers) handleCatalog(w http.ResponseWriter, r *http.Request) {
	base := requestBase(r)
	w.Header().Set("Content-Type", "application/linkset+json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"linkset": []any{
			map[string]any{
				"anchor": base,
				"service-desc": []any{
					map[string]any{
						"href": base + "/.well-known/openapi",
						"type": "application/openapi+json",
					},
				},
			},
		},
	})
}

func (h *handlers) handleAIPlugin(w http.ResponseWriter, r *http.Request) {
	base := requestBase(r)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"schema_version":        "v1",
		"name_for_human":        "Morsel",
		"name_for_model":        "morsel",
		"description_for_human": "Control plane for the Morsel self-hosted PaaS platform.",
		"description_for_model": "API for deploying and managing containerised HTTP, worker, and cron apps on a self-hosted Kubernetes-backed PaaS. Supports token-based auth, async deploy operations, app hibernation, cost tracking, and operator approval flows.",
		"auth": map[string]any{
			"type": "bearer",
		},
		"api": map[string]any{
			"type": "openapi",
			"url":  base + "/.well-known/openapi",
		},
	})
}

// requestBase derives the scheme and host from the incoming request.
// It honours X-Forwarded-Proto for deployments behind a reverse proxy.
func requestBase(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil {
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			scheme = proto
		} else {
			scheme = "http"
		}
	}
	return scheme + "://" + r.Host
}
