package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/adminui/pages"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
)

// acceptedOperationBody is the subset of AcceptedOperation the admin UI needs
// to redirect to the operation-status page.
type acceptedOperationBody struct {
	OperationID string `json:"operation_id"`
}

// apiErrorBody mirrors the {"error": {...}} shape written by writeJSONError.
type apiErrorBody struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// decodeAcceptedOperationID reads and closes resp's body, returning the
// operation ID if resp is a 202 Accepted with a well-formed body.
func decodeAcceptedOperationID(resp *http.Response) (string, bool) {
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusAccepted {
		return "", false
	}
	var body acceptedOperationBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil || body.OperationID == "" {
		return "", false
	}
	return body.OperationID, true
}

// redirectToAppOperation inspects the response from an async app action
// (hibernate/wake/delete). On 202 Accepted it redirects to the operation
// status page, which polls until the operation settles before returning to
// returnURL. On any other outcome it redirects straight back to returnURL
// with a flash message — there's nothing to poll.
func (h *Handler) redirectToAppOperation(w http.ResponseWriter, r *http.Request, resp *http.Response, err error, slug, appName, label, returnURL string) {
	ctx := r.Context()
	if err != nil {
		http.Redirect(w, r, returnURL+"?flash_error=1&flash="+url.QueryEscape("Failed to reach the platform API: "+err.Error()), http.StatusSeeOther)
		return
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			ctxlog.From(ctx).Warn("close response body", "err", closeErr)
		}
	}()

	if resp.StatusCode == http.StatusAccepted {
		var body acceptedOperationBody
		if decErr := json.NewDecoder(resp.Body).Decode(&body); decErr != nil || body.OperationID == "" {
			ctxlog.From(ctx).Warn("decode accepted operation body", "err", decErr)
			http.Redirect(w, r, returnURL+"?flash_error=1&flash="+url.QueryEscape("Started, but couldn't track progress."), http.StatusSeeOther)
			return
		}
		org, repo, _ := strings.Cut(slug, "/")
		dest := "/apps/" + org + "/" + repo + "/" + appName + "/operations/" + body.OperationID +
			"?label=" + url.QueryEscape(label) + "&return=" + url.QueryEscape(returnURL)
		http.Redirect(w, r, dest, http.StatusSeeOther)
		return
	}

	var errBody apiErrorBody
	message := "request failed (HTTP " + resp.Status + ")"
	if decErr := json.NewDecoder(resp.Body).Decode(&errBody); decErr == nil && errBody.Error.Message != "" {
		message = errBody.Error.Message
	}
	http.Redirect(w, r, returnURL+"?flash_error=1&flash="+url.QueryEscape(message), http.StatusSeeOther)
}

// ServeAppOperationStatus handles GET /apps/{org}/{repo}/{appName}/operations/{opID}.
// It renders a page that polls the operation until it settles, then redirects
// to the return URL (default /apps) with a flash summary.
func (h *Handler) ServeAppOperationStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slug, appName := appParams(r)
	opID := r.PathValue("opID")

	returnURL := r.URL.Query().Get("return")
	if returnURL == "" {
		returnURL = "/apps"
	}
	label := r.URL.Query().Get("label")
	if label == "" {
		label = "Working…"
	}

	data := pages.OperationStatusPageData{
		Title:  label,
		Return: returnURL,
		Rows: []pages.OperationRow{
			{Name: appName, StatusURL: appOperationStatusPath(slug, appName, opID)},
		},
	}
	if err := pages.OperationStatusPage(data).Render(ctx, w); err != nil {
		ctxlog.From(ctx).Warn("render operation status page", "err", err)
	}
}

// HandleAppOperationStatusJSON handles GET /apps/{org}/{repo}/{appName}/operations/{opID}/status.
// It proxies the control plane's operation status JSON so the browser-side
// poller never needs the session's bearer token.
func (h *Handler) HandleAppOperationStatusJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slug, appName := appParams(r)
	org, repo, _ := strings.Cut(slug, "/")
	opID := r.PathValue("opID")
	path := "/api/repos/" + org + "/" + repo + "/apps/" + appName + "/operations/" + opID

	resp, err := h.apiGet(ctx, path)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			ctxlog.From(ctx).Warn("close response body", "err", closeErr)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		ctxlog.From(ctx).Warn("copy operation status response", "err", err)
	}
}

// ServeRepoOperationsStatus handles GET /repos/{org}/{repo}/operations.
// It tracks multiple operations at once (e.g. one per app in a "delete all"
// batch), keyed by repeated app=/op= query parameter pairs.
func (h *Handler) ServeRepoOperationsStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slug := repoSlug(r)

	returnURL := r.URL.Query().Get("return")
	if returnURL == "" {
		returnURL = "/repos"
	}
	label := r.URL.Query().Get("label")
	if label == "" {
		label = "Working…"
	}

	apps := r.URL.Query()["app"]
	ops := r.URL.Query()["op"]
	rows := make([]pages.OperationRow, 0, len(ops))
	for i := 0; i < len(apps) && i < len(ops); i++ {
		rows = append(rows, pages.OperationRow{
			Name:      apps[i],
			StatusURL: appOperationStatusPath(slug, apps[i], ops[i]),
		})
	}

	if len(rows) == 0 {
		http.Redirect(w, r, returnURL, http.StatusSeeOther)
		return
	}

	data := pages.OperationStatusPageData{
		Title:  label,
		Return: returnURL,
		Rows:   rows,
	}
	if err := pages.OperationStatusPage(data).Render(ctx, w); err != nil {
		ctxlog.From(ctx).Warn("render operation status page", "err", err)
	}
}

// appOperationStatusPath builds the admin UI's JSON status endpoint for a
// single app operation, scoped under the same route the browser is on.
func appOperationStatusPath(slug, appName, opID string) string {
	org, repo, _ := strings.Cut(slug, "/")
	return "/apps/" + org + "/" + repo + "/" + appName + "/operations/" + opID + "/status"
}
