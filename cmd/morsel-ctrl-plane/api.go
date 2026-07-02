package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/api"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/db"
	dbqueries "github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/db/queries"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/hibernation"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/platform"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/platforms"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/store"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
	"github.com/ifeanyiecheruo/morsel/internal/kube"
)

func runAPI(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("api", flag.ExitOnError)
	addr := fs.String("addr", ":8080", "HTTP listen address")
	platformName := fs.String("platform", "local", "platform implementation (local|gcp)")
	dbPath := fs.String("db", "morsel.db", "SQLite database path")
	kubeconfigPath := fs.String("kubeconfig", "", "path to kubeconfig file (defaults to in-cluster config, then ~/.kube/config)")
	_ = fs.Parse(args)

	logger := ctxlog.From(ctx)

	s, closeStore := initializeStore(ctx, logger, *dbPath)
	defer closeStore()

	plat := initializePlatform(ctx, logger, *platformName, s)
	kubeClient := initializeKube(logger, *kubeconfigPath)

	go runCertRenewal(ctx, plat, kubeClient, logger)
	go hibernation.New(s, kubeClient, plat, 0).Run(ctx)
	go runPriceFetch(ctx, plat, s, logger)

	runServer(ctx, *addr, 30*time.Second, func() *http.Server {
		return &http.Server{Handler: api.NewMux(ctx, plat, s, kubeClient)}
	})
}

func initializeStore(ctx context.Context, logger *slog.Logger, dbPath string) (*store.Store, func()) {
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
	return store.New(dbqueries.New(database)), closeStore
}

func initializePlatform(ctx context.Context, logger *slog.Logger, platformName string, s *store.Store) platform.Platform {
	plat, err := platforms.Create(platformName, s)
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
	return plat
}

func runCertRenewal(ctx context.Context, plat platform.Platform, kubeClient *kube.Client, logger *slog.Logger) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			checkAndRenewCert(ctx, plat, kubeClient, logger)
		}
	}
}

func checkAndRenewCert(ctx context.Context, plat platform.Platform, kubeClient *kube.Client, logger *slog.Logger) {
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

func runPriceFetch(ctx context.Context, plat platform.Platform, s *store.Store, logger *slog.Logger) {
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
	fetch() // fetch immediately on startup
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

func initializeKube(logger *slog.Logger, kubeconfigPath string) *kube.Client {
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
	return deployer
}
