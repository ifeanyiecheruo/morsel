package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/api"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/db"
	dbqueries "github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/db/queries"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/platform"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/platforms"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/store"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/watchers"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
	"github.com/ifeanyiecheruo/morsel/internal/health"
	"github.com/ifeanyiecheruo/morsel/internal/kube"
)

func newAPICmd(ctx context.Context) *cobra.Command {
	var addr string
	var platformName string
	var dbPath string
	var kubeconfigPath string
	var githubClientID string
	var gatewayPort int

	cmd := &cobra.Command{
		Use:   "api",
		Short: "Run the control-plane REST API server",
		RunE: func(cmd *cobra.Command, _ []string) error {
			reporter, receiver := health.NewReporter()
			ctx = health.With(ctx, reporter)
			go receiver.Run(ctx)

			storeInstance, closeStore := initializeStore(ctx, dbPath)
			defer closeStore()

			plat := initializePlatform(ctx, platformName, storeInstance, gatewayPort)
			kubeClient := initializeKube(ctx, kubeconfigPath)

			go runCertRenewal(ctx, plat, kubeClient)
			go watchers.NewHibernation(storeInstance, kubeClient, plat, 0).Run(ctx)
			go watchers.NewBudget(storeInstance, kubeClient, plat, 0).Run(ctx)
			go runPriceFetch(ctx, plat, storeInstance)

			apiH := api.NewMux(ctx, plat, storeInstance, kubeClient, receiver, githubClientID)
			runServer(ctx, addr, 30*time.Second, func() *http.Server {
				return &http.Server{Handler: apiH}
			})
			return nil
		},
	}

	cmd.Flags().StringVar(&addr, "addr", ":8080", "HTTP listen address")
	cmd.Flags().StringVar(&platformName, "platform", "local", "platform implementation (local|gcp)")
	cmd.Flags().StringVar(&dbPath, "db", "morsel.db", "SQLite database path")
	cmd.Flags().StringVar(&kubeconfigPath, "kubeconfig", "", "path to kubeconfig file (defaults to in-cluster config, then ~/.kube/config)")
	cmd.Flags().StringVar(&githubClientID, "github-client-id", "", "GitHub OAuth App client ID (enables Device Flow login)")
	cmd.Flags().IntVar(&gatewayPort, "gateway-port", 443, "host port the HTTPS app gateway listens on")

	return cmd
}

func initializeStore(ctx context.Context, dbPath string) (*store.Store, func()) {
	logger := ctxlog.From(ctx)
	reporter, err := health.From(ctx)
	if err != nil {
		logger.Error("failed to get health reporter", "err", err)
		os.Exit(1)
	}

	storeHealth := reporter.NewComponent("database", true)

	database, err := db.Open(ctx, dbPath)
	if err != nil {
		logger.Error("database error", "err", err)
		os.Exit(1)
	}
	if err := db.Migrate(ctx, database); err != nil {
		logger.Error("migration error", "err", err)
		os.Exit(1)
	}
	closeStore := func() {
		if err := database.Close(); err != nil {
			logger.Error("database close error", "err", err)
		}
	}

	result := store.New(dbqueries.New(database), database)
	storeHealth.Report(true, "ready")

	return result, closeStore
}

func initializePlatform(ctx context.Context, platformName string, s *store.Store, gatewayPort int) platform.Platform {
	logger := ctxlog.From(ctx)
	reporter, err := health.From(ctx)
	if err != nil {
		logger.Error("failed to get health reporter", "err", err)
		os.Exit(1)
	}

	platformHealth := reporter.NewComponent("platform", true)

	plat, err := platforms.Create(platformName, s, gatewayPort)
	if err != nil {
		logger.Error("platform error", "err", err)
		os.Exit(1)
	}
	if err := plat.Secrets().Migrate(ctx); err != nil {
		logger.Error("secret migration error", "err", err)
		os.Exit(1)
	}
	if seeder, ok := plat.(platform.Seeder); ok {
		if err := seeder.SeedDefaults(ctx); err != nil {
			logger.Error("seed defaults error", "err", err)
			os.Exit(1)
		}
	}

	platformHealth.Report(true, "ready")

	return plat
}

func runCertRenewal(ctx context.Context, plat platform.Platform, kubeClient *kube.Client) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			checkAndRenewCert(ctx, plat, kubeClient)
		}
	}
}

func checkAndRenewCert(ctx context.Context, plat platform.Platform, kubeClient *kube.Client) {
	logger := ctxlog.From(ctx)
	ns := plat.Namespace()
	expiry, err := kubeClient.GetTLSCertExpiry(ctx, ns, kube.MorselTLSSecret)
	if err != nil {
		logger.Error("check cert expiry", "err", err)
		return
	}
	if expiry == nil {
		return
	}
	if time.Until(*expiry) >= 30*24*time.Hour {
		return
	}
	logger.Info("renewing tls cert", "expires", *expiry)
	cert, err := plat.Certs().Renew(ctx, plat.BaseDomain(), 30*24*time.Hour)
	if err != nil {
		logger.Error("renew tls cert", "err", err)
		return
	}
	if err := kubeClient.StoreTLSSecret(ctx, ns, kube.MorselTLSSecret, cert); err != nil {
		logger.Error("store renewed tls cert", "err", err)
		return
	}
	logger.Info("tls cert renewed")
}

func initializeKube(ctx context.Context, kubeconfigPath string) *kube.Client {
	logger := ctxlog.From(ctx)
	reporter, err := health.From(ctx)
	if err != nil {
		logger.Error("failed to get health reporter", "err", err)
		os.Exit(1)
	}

	k8sHealth := reporter.NewComponent("kubernetes", true)

	deployer, err := kube.New(kubeconfigPath)
	if err != nil {
		var ce *kube.ConfigError
		switch {
		case errors.As(err, &ce) && ce.KubeconfigPath != "":
			logger.Error("kubernetes config not found",
				"err", err,
				"kubeconfig", ce.KubeconfigPath,
				"remedy", "verify the file exists, is readable, and contains a valid kubeconfig")
		case errors.As(err, &ce):
			logger.Error("kubernetes config not found",
				"err", err,
				"remedy", "run inside a Kubernetes pod with a mounted service account, "+
					"set the KUBECONFIG environment variable, "+
					"ensure ~/.kube/config exists, "+
					"or pass --kubeconfig pointing to a valid kubeconfig file")
		default:
			logger.Error("kubernetes client unavailable",
				"err", err,
				"remedy", "verify the cluster API server address in the kubeconfig is correct and reachable")
		}
		os.Exit(1)
	}

	k8sHealth.Report(true, "ready")
	return deployer
}

func runPriceFetch(ctx context.Context, plat platform.Platform, s *store.Store) {
	logger := ctxlog.From(ctx)
	fetch := func() {
		prices, err := plat.Pricing().Prices(ctx)
		if err != nil {
			logger.Error("price fetch failed", "err", err)
			return
		}
		if _, err := s.InsertPriceSnapshot(ctx, prices.ComputeCPUPerCoreHour, prices.ComputeMemPerGBHour, prices.StoragePerGBMonth, prices.RegistryPerGBMonth, prices.FetchedAt); err != nil {
			logger.Error("store price snapshot", "err", err)
		}
	}
	fetch()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fetch()
		}
	}
}
