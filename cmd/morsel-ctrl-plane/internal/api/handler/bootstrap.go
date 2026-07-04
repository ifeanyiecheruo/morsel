package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"golang.org/x/crypto/bcrypt"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/platform"
)

type bootstrapRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// HandleBootstrap creates the first operator principal. It is called by
// 'morsel service deploy' immediately after provisioning and requires the
// one-time X-Bootstrap-Token written to the cluster before the pod started.
// Returns 409 Conflict if any operators already exist so it succeeds exactly once.
func (h *Handler) HandleBootstrap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	existing, err := h.store.ListPrincipals(ctx)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if len(existing) > 0 {
		http.Error(w, "already bootstrapped", http.StatusConflict)
		return
	}

	// Validate the one-time bootstrap token before accepting credentials.
	token := r.Header.Get("X-Bootstrap-Token")
	expected, err := h.plat.Secrets().GetBootstrapToken(ctx)
	if errors.Is(err, platform.ErrSecretNotFound) || token == "" || token != expected {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var req bootstrapRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Username == "" || req.Password == "" {
		http.Error(w, "username and password required", http.StatusBadRequest)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if err := h.store.AddPrincipalWithPasswordHash(ctx, req.Username, string(hash)); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Token is single-use — delete it so it cannot be replayed.
	_ = h.plat.Secrets().DeleteBootstrapToken(ctx)

	w.WriteHeader(http.StatusCreated)
}
