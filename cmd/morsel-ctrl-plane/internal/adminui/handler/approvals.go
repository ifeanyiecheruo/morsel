package handler

import (
	"net/http"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/adminui/pages"
)

// ServeApprovals handles GET /approvals.
func (h *Handler) ServeApprovals(w http.ResponseWriter, r *http.Request) {
	_ = pages.ApprovalsPage(pages.ApprovalsPageData{}).Render(r.Context(), w)
}

// HandleApprovalsBatch handles POST /approvals/batch.
func (h *Handler) HandleApprovalsBatch(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/approvals", http.StatusSeeOther)
}
