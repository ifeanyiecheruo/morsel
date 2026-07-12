package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
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

				serviceAddr, retryAfter, err := wakeProxyWakeApp(r.Context(), ctrlPlane, host, token)
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

				target, err := url.Parse(serviceAddr)
				if err != nil {
					rlog.Error("parse service addr", "addr", serviceAddr, "err", err)
					http.Error(w, "invalid service address", http.StatusInternalServerError)
					return
				}

				proxy := httputil.NewSingleHostReverseProxy(target)
				proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
					ctxlog.From(r.Context()).Error("proxy", "host", host, "target", serviceAddr, "err", err)
					http.Error(w, "upstream unavailable", http.StatusBadGateway)
				}
				proxy.ServeHTTP(w, r)
			})

			proxyHealth.Report(true, "ready")
			logger.Info("wake proxy starting", "api", ctrlPlane)
			runServer(ctx, addr, 30*time.Second, func() *http.Server {
				return &http.Server{
					Handler:      mux,
					ReadTimeout:  10 * time.Minute,
					WriteTimeout: 10 * time.Minute,
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

type wakeProxyResponse struct {
	ServiceAddr string `json:"service_addr"`
}

func wakeProxyWakeApp(ctx context.Context, ctrlPlane, host, token string) (serviceAddr, retryAfter string, _ error) {
	wakeURL := ctrlPlane + "/internal/wake?host=" + url.QueryEscape(host)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, wakeURL, http.NoBody)
	if err != nil {
		return "", "", fmt.Errorf("build wake request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 6 * time.Minute}
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
	if resp.StatusCode != http.StatusOK {
		return "", resp.Header.Get("Retry-After"), fmt.Errorf("wake returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var wr wakeProxyResponse
	if err := json.Unmarshal(body, &wr); err != nil {
		return "", "", fmt.Errorf("decode wake response: %w", err)
	}
	if wr.ServiceAddr == "" {
		return "", "", fmt.Errorf("wake response missing service_addr")
	}
	return wr.ServiceAddr, "", nil
}
