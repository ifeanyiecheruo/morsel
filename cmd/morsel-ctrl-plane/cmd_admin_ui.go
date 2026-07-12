package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/adminui"
	"github.com/ifeanyiecheruo/morsel/internal/health"
)

func newAdminUICmd(ctx context.Context) *cobra.Command {
	var addr string
	var apiURL string
	var sessionSecretHex string
	var githubClientID string
	var githubClientSecret string

	cmd := &cobra.Command{
		Use:   "admin-ui",
		Short: "Run the admin UI as a standalone service",
		Long: `Run the Morsel admin UI as a standalone HTTP server. All data is
fetched from the control-plane REST API; the admin UI has no direct
access to the database or Kubernetes.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if apiURL == "" {
				return fmt.Errorf("--api-url is required")
			}

			var sessionKey []byte
			if sessionSecretHex != "" {
				var err error
				sessionKey, err = hex.DecodeString(sessionSecretHex)
				if err != nil {
					return fmt.Errorf("decode --session-secret: %w", err)
				}
				if len(sessionKey) < 32 {
					return fmt.Errorf("--session-secret must be at least 32 bytes (64 hex chars)")
				}
			} else {
				// Generate an ephemeral key — sessions are lost on restart.
				sessionKey = make([]byte, 32)
				if _, err := rand.Read(sessionKey); err != nil {
					return fmt.Errorf("generate session key: %w", err)
				}
				fmt.Fprintln(os.Stderr, "warning: --session-secret not set; using ephemeral key — sessions will not survive restart")
			}

			reporter, receiver := health.NewReporter()
			ctx = health.With(ctx, reporter)
			go receiver.Run(ctx)
			uiHealth := reporter.NewComponent("ui", true)

			httpClient := &http.Client{Timeout: 30 * time.Second}
			h := adminui.NewMux(ctx, apiURL, httpClient, sessionKey, githubClientID, githubClientSecret, receiver)
			uiHealth.Report(true, "ready")

			runServer(ctx, addr, 30*time.Second, func() *http.Server {
				return &http.Server{Handler: h}
			})
			return nil
		},
	}

	cmd.Flags().StringVar(&addr, "addr", ":8090", "HTTP listen address for the admin UI")
	cmd.Flags().StringVar(&apiURL, "api-url", "", "base URL of the Morsel control-plane REST API (required)")
	cmd.Flags().StringVar(&sessionSecretHex, "session-secret", "", "hex-encoded 32-byte key for signing session cookies (auto-generated if omitted)")
	cmd.Flags().StringVar(&githubClientID, "github-client-id", "", "GitHub OAuth App client ID")
	cmd.Flags().StringVar(&githubClientSecret, "github-client-secret", "", "GitHub OAuth App client secret")

	return cmd
}
