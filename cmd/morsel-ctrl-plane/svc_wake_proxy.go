package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
	"github.com/ifeanyiecheruo/morsel/internal/health"
	"github.com/ifeanyiecheruo/morsel/internal/kube"
)

// readToken reads the projected service account token from disk. The file is
// updated in place by kubelet when the token is rotated, so we read it fresh
// on every request rather than caching the value at startup.
func readToken(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read token file %s: %w", path, err)
	}
	return strings.TrimSpace(string(b)), nil
}

//go:embed wake-proxy-wait.html
var waitPageHTML string

// waitPageTmpl is the interstitial shown while an app wakes from hibernation.
// The control plane kicks off the scale-up and returns immediately (see
// wakeProxyWakeApp), so this page — not a held connection — is what carries
// the wait: the meta-refresh retries the request every 5s until the app's
// HTTPRoute has been restored and the gateway routes straight to it,
// bypassing this proxy entirely.
var waitPageTmpl = template.Must(template.New("wait").Parse(waitPageHTML))

type waitPageData struct {
	App string
}

func newWakeProxyCmd(ctx context.Context) *cobra.Command {
	var addr string
	var apiURL string
	var tokenFile string

	cmd := &cobra.Command{
		Use:   "wake-proxy",
		Short: "Run the hibernation wake proxy",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if apiURL == "" {
				return fmt.Errorf("--api is required")
			}
			ctrlPlane := strings.TrimRight(apiURL, "/")
			logger := ctxlog.From(ctx)

			reporter, receiver := health.NewReporter()
			ctx = health.With(ctx, reporter)
			go receiver.Run(ctx)

			proxyHealth := reporter.NewComponent("proxy", true)

			mux := http.NewServeMux()
			mux.HandleFunc("GET /livez", receiver.LivezHandler)
			mux.HandleFunc("GET /readyz", receiver.ReadyzHandler)
			mux.HandleFunc("GET /healthz", receiver.HealthzHandler)
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				rlog := ctxlog.From(r.Context())
				host := r.Host
				if host == "" {
					http.Error(w, "missing Host header", http.StatusBadRequest)
					return
				}

				token, err := readToken(tokenFile)
				if err != nil {
					rlog.Error("read wake token", "err", err)
					http.Error(w, "token unavailable", http.StatusInternalServerError)
					return
				}

				appName, retryAfter, err := wakeProxyWakeApp(r.Context(), ctrlPlane, host, token)
				if err != nil {
					rlog.Error("wake app", "host", host, "err", err)
					if retryAfter != "" {
						w.Header().Set("Retry-After", retryAfter)
						http.Error(w, "platform is over budget for this period", http.StatusServiceUnavailable)
					} else {
						http.Error(w, fmt.Sprintf("wake failed: %v", err), http.StatusServiceUnavailable)
					}
					return
				}
				if appName == "" {
					// Fall back to a best-effort label parsed from the hostname
					// ("app.repo.app.domain") if the control plane didn't send one.
					appName, _, _ = strings.Cut(host, ".")
				}

				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				if err := waitPageTmpl.Execute(w, waitPageData{App: appName}); err != nil {
					rlog.Error("render wait page", "err", err)
				}
			})

			proxyHealth.Report(true, "ready")
			logger.Info("wake proxy starting", "api", ctrlPlane)
			runServer(ctx, addr, 30*time.Second, func() *http.Server {
				return &http.Server{
					Handler:      mux,
					ReadTimeout:  10 * time.Second,
					WriteTimeout: 10 * time.Second,
				}
			})
			return nil
		},
	}

	cmd.Flags().StringVar(&addr, "addr", ":8080", "HTTP listen address")
	cmd.Flags().StringVar(&apiURL, "api", "", "base URL of the Morsel control-plane API (required)")
	cmd.Flags().StringVar(&tokenFile, "token-file", kube.WakeProxyTokenPath, "path to the projected service account token file")

	return cmd
}

// wakeAckResponse mirrors the control plane's immediate response to
// /internal/wake — it no longer waits for the app to become ready, so there's
// no service address to forward to; just an app name for the wait page.
type wakeAckResponse struct {
	Status string `json:"status"`
	App    string `json:"app"`
}

// wakeProxyWakeApp asks the control plane to wake the app for host. The
// control plane kicks off the scale-up in the background and responds
// immediately, so this call is fast — it does not wait for the app to
// actually become ready.
func wakeProxyWakeApp(ctx context.Context, ctrlPlane, host, token string) (appName, retryAfter string, _ error) {
	wakeURL := ctrlPlane + "/internal/wake?host=" + url.QueryEscape(host)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, wakeURL, http.NoBody)
	if err != nil {
		return "", "", fmt.Errorf("build wake request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("POST wake: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			ctxlog.From(ctx).Error("close wake response body", "err", err)
		}
	}()
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		ctxlog.From(ctx).Warn("read wake response body", "err", readErr)
	}
	if resp.StatusCode != http.StatusAccepted {
		return "", resp.Header.Get("Retry-After"), fmt.Errorf("wake returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var ack wakeAckResponse
	if err := json.Unmarshal(body, &ack); err != nil {
		return "", "", fmt.Errorf("decode wake response: %w", err)
	}
	return ack.App, "", nil
}
