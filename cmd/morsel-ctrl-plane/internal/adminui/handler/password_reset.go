package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/adminui/pages"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
)

type changePasswordReq struct {
	NewPassword string `json:"new_password"`
}

// ServePasswordReset handles GET /password-reset.
func (h *Handler) ServePasswordReset(w http.ResponseWriter, r *http.Request) {
	_ = pages.PasswordResetPage("").Render(r.Context(), w)
}

// HandlePasswordReset handles POST /password-reset.
func (h *Handler) HandlePasswordReset(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		_ = pages.PasswordResetPage("Bad request.").Render(r.Context(), w)
		return
	}
	newPassword := r.FormValue("new_password")
	confirm := r.FormValue("confirm_password")
	if newPassword == "" {
		_ = pages.PasswordResetPage("Password cannot be empty.").Render(r.Context(), w)
		return
	}
	if len(newPassword) < 8 {
		_ = pages.PasswordResetPage("Password must be at least 8 characters.").Render(r.Context(), w)
		return
	}
	if newPassword != confirm {
		_ = pages.PasswordResetPage("Passwords do not match.").Render(r.Context(), w)
		return
	}

	body, _ := json.Marshal(changePasswordReq{NewPassword: newPassword})
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, h.apiURL+"/api/operator/password", bytes.NewReader(body))
	if err != nil {
		_ = pages.PasswordResetPage("Failed to submit. Try again.").Render(r.Context(), w)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	s := sessionFromContext(r.Context())
	if s != nil {
		req.Header.Set("Authorization", "Bearer "+s.AccessToken)
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		_ = pages.PasswordResetPage("Failed to submit. Try again.").Render(r.Context(), w)
		return
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			ctxlog.From(r.Context()).Warn("close response body", "err", closeErr)
		}
	}()

	if resp.StatusCode == http.StatusBadRequest {
		var errBody struct {
			Error struct{ Message string } `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		msg := errBody.Error.Message
		if msg == "" {
			msg = "Invalid request."
		}
		_ = pages.PasswordResetPage(strings.TrimSpace(msg)).Render(r.Context(), w)
		return
	}
	if resp.StatusCode != http.StatusNoContent {
		_ = pages.PasswordResetPage("Password change failed. Try again.").Render(r.Context(), w)
		return
	}

	// Clear the password-reset flag from the session cookie.
	if s != nil {
		s.PasswordResetRequired = false
		_ = h.setSessionCookie(w, s)
	}
	http.Redirect(w, r, "/apps", http.StatusSeeOther)
}
