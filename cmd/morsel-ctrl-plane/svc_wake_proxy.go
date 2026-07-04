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
)

func newWakeProxyCmd(ctx context.Context) *cobra.Command {
	var addr string
	var apiURL string

	cmd := &cobra.Command{
		Use:   "wake-proxy",
		Short: "Run the hibernation wake proxy",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if apiURL == "" {
				return fmt.Errorf("--api is required")
			}
			ctrlPlane := strings.TrimRight(apiURL, "/")
			logger := ctxlog.From(ctx)

			token := os.Getenv("WAKE_PROXY_TOKEN")
			if token == "" {
				return fmt.Errorf("WAKE_PROXY_TOKEN environment variable is required")
			}

			mux := http.NewServeMux()
			mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
			})
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				rlog := ctxlog.From(r.Context())
				host := r.Host
				if host == "" {
					http.Error(w, "missing Host header", http.StatusBadRequest)
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
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
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
	body, _ := io.ReadAll(resp.Body)
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
