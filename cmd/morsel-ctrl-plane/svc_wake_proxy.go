package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
)

func runWakeProxy(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("svc wake-proxy", flag.ExitOnError)
	addr := fs.String("addr", ":8080", "HTTP listen address")
	apiURL := fs.String("api", "", "base URL of the Morsel control-plane API (required)")
	_ = fs.Parse(args)

	if *apiURL == "" {
		fmt.Fprintln(os.Stderr, "morsel-ctrl-plane svc wake-proxy: --api is required")
		os.Exit(2)
	}
	ctrlPlane := strings.TrimRight(*apiURL, "/")
	logger := ctxlog.From(ctx)

	token := os.Getenv("WAKE_PROXY_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "morsel-ctrl-plane svc wake-proxy: WAKE_PROXY_TOKEN is required")
		os.Exit(2)
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
	runServer(ctx, *addr, 30*time.Second, func() *http.Server {
		return &http.Server{
			Handler:      mux,
			ReadTimeout:  10 * time.Minute,
			WriteTimeout: 10 * time.Minute,
		}
	})
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
