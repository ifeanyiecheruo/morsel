package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ifeanyiecheruo/morsel/internal/api"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
	"github.com/ifeanyiecheruo/morsel/internal/db"
	dbqueries "github.com/ifeanyiecheruo/morsel/internal/db/queries"
	"github.com/ifeanyiecheruo/morsel/internal/kube"
	"github.com/ifeanyiecheruo/morsel/internal/platform"
	"github.com/ifeanyiecheruo/morsel/internal/platforms"
	"github.com/ifeanyiecheruo/morsel/internal/store"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	platformName := flag.String("platform", "local", "platform implementation (local|gcp)")
	dbPath := flag.String("db", "morsel.db", "SQLite database path")
	kubeconfigPath := flag.String("kubeconfig", "", "path to kubeconfig file (defaults to in-cluster config, then ~/.kube/config)")
	flag.Parse()

	logger := slog.Default()
	ctx := ctxlog.With(context.Background(), logger)

	s, closeStore := initializeStore(ctx, logger, *dbPath)
	defer closeStore()

	plat := initializePlatform(ctx, logger, *platformName, s)
	kubeClient := initializeKube(logger, *kubeconfigPath)

	go runCertRenewal(ctx, plat, kubeClient, logger)

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		logger.Error("listen error", "err", err)
		os.Exit(1)
	}
	logger.Info("listening", "addr", ln.Addr())

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGHUP, os.Interrupt)

	for {
		srv := &http.Server{Handler: api.NewMux(ctx, plat, s, kubeClient)}

		go func() {
			if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
				logger.Error("server error", "err", err)
				os.Exit(1)
			}
		}()

		sig := <-quit
		logger.Info("signal received", "signal", sig)

		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if err := srv.Shutdown(ctx); err != nil {
			logger.Error("shutdown error", "err", err)
		}
		cancel()

		if sig == syscall.SIGHUP {
			logger.Info("reloading")
			continue
		}

		if err := ln.Close(); err != nil {
			logger.Error("listener close error", "err", err)
		}
		logger.Info("shutdown complete")
		return
	}
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
