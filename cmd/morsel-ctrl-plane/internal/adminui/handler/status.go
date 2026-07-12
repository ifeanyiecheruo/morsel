package handler

import (
	"encoding/json"
	"net/http"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/adminui/pages"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
)

type apiStatusResponse struct {
	Certs *struct {
		ExpiringSoon []string `json:"expiring_soon"`
	} `json:"certs"`
}

// ServeStatus handles GET /status.
func (h *Handler) ServeStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	resp, err := h.apiGet(ctx, "/api/operator/status")
	if err != nil || resp.StatusCode == http.StatusUnauthorized {
		if resp != nil {
			if closeErr := resp.Body.Close(); closeErr != nil {
				ctxlog.From(ctx).Warn("close response body", "err", closeErr)
			}
		}
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			ctxlog.From(ctx).Warn("close response body", "err", closeErr)
		}
	}()

	var statusData apiStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&statusData); err != nil {
		ctxlog.From(ctx).Warn("decode status response", "err", err)
	}

	certExpiringSoon := statusData.Certs != nil && len(statusData.Certs.ExpiringSoon) > 0
	certDomain := ""
	if certExpiringSoon {
		certDomain = statusData.Certs.ExpiringSoon[0]
	}

	if err := pages.StatusPage(pages.StatusPageData{
		ClusterHealthy:      true,
		ControlPlaneHealthy: resp.StatusCode == http.StatusOK,
		CertExpiringSoon:    certExpiringSoon,
		CertDomain:          certDomain,
	}).Render(ctx, w); err != nil {
		ctxlog.From(ctx).Warn("render status page", "err", err)
	}
}
