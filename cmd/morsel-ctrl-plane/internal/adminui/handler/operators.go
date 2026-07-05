package handler

import (
	"encoding/json"
	"net/http"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/adminui/pages"
)

type operatorPrincipalDetail struct {
	Username              string `json:"username"`
	PasswordResetRequired bool   `json:"password_reset_required"`
	IsAdmin               bool   `json:"is_admin"`
}

type operatorPrincipalsResponse struct {
	Principals []operatorPrincipalDetail `json:"principals"`
}

// ServeOperators handles GET /operators.
func (h *Handler) ServeOperators(w http.ResponseWriter, r *http.Request) {
	resp, err := h.apiGet(r.Context(), "/api/operator/principals")
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			_ = resp.Body.Close()
		}
		_ = pages.OperatorsPage(pages.OperatorsPageData{Flash: "Failed to load operators."}).Render(r.Context(), w)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	var body operatorPrincipalsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		_ = pages.OperatorsPage(pages.OperatorsPageData{Flash: "Failed to parse response."}).Render(r.Context(), w)
		return
	}

	rows := make([]pages.OperatorRow, len(body.Principals))
	for i, p := range body.Principals {
		rows[i] = pages.OperatorRow{
			Username:              p.Username,
			PasswordResetRequired: p.PasswordResetRequired,
			IsAdmin:               p.IsAdmin,
		}
	}
	_ = pages.OperatorsPage(pages.OperatorsPageData{
		Rows:          rows,
		ViewerIsAdmin: sessionIsAdmin(r.Context()),
	}).Render(r.Context(), w)
}

// HandleRequirePasswordReset handles POST /operators/{principal}/require-password-reset.
func (h *Handler) HandleRequirePasswordReset(w http.ResponseWriter, r *http.Request) {
	principal := r.PathValue("principal")
	resp, err := h.apiPost(r.Context(), "/api/operator/principals/"+principal+"/require-password-reset", nil)
	if err != nil || (resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK) {
		if resp != nil {
			_ = resp.Body.Close()
		}
		http.Redirect(w, r, "/operators?flash=error", http.StatusSeeOther)
		return
	}
	_ = resp.Body.Close()
	http.Redirect(w, r, "/operators", http.StatusSeeOther)
}

// HandleInvalidatePrincipalPassword handles POST /operators/{principal}/invalidate-password.
func (h *Handler) HandleInvalidatePrincipalPassword(w http.ResponseWriter, r *http.Request) {
	principal := r.PathValue("principal")
	resp, err := h.apiPost(r.Context(), "/api/operator/principals/"+principal+"/invalidate-password", nil)
	if err != nil || (resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK) {
		if resp != nil {
			_ = resp.Body.Close()
		}
		http.Redirect(w, r, "/operators?flash=error", http.StatusSeeOther)
		return
	}
	_ = resp.Body.Close()
	http.Redirect(w, r, "/operators", http.StatusSeeOther)
}

type setPrincipalPasswordReq struct {
	NewPassword string `json:"new_password"`
	Invalidate  bool   `json:"invalidate,omitempty"`
}

// HandleSetPrincipalPassword handles POST /operators/{principal}/set-password.
func (h *Handler) HandleSetPrincipalPassword(w http.ResponseWriter, r *http.Request) {
	principal := r.PathValue("principal")
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/operators?flash=error", http.StatusSeeOther)
		return
	}
	newPassword := r.FormValue("new_password")
	invalidate := r.FormValue("invalidate") == "true"

	resp, err := h.apiPost(r.Context(), "/api/operator/principals/"+principal+"/set-password", setPrincipalPasswordReq{
		NewPassword: newPassword,
		Invalidate:  invalidate,
	})
	if err != nil || (resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK) {
		if resp != nil {
			_ = resp.Body.Close()
		}
		http.Redirect(w, r, "/operators?flash=error", http.StatusSeeOther)
		return
	}
	_ = resp.Body.Close()
	http.Redirect(w, r, "/operators", http.StatusSeeOther)
}
