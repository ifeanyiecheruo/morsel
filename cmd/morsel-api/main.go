package main

import (
	"context"
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
	"github.com/ifeanyiecheruo/morsel/internal/platform"
	"github.com/ifeanyiecheruo/morsel/internal/platforms"
	"github.com/ifeanyiecheruo/morsel/internal/secrets"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	platformName := flag.String("platform", "local", "platform implementation (local|gcp)")
	dbPath := flag.String("db", "morsel.db", "SQLite database path")
	flag.Parse()

	logger := slog.Default()
	ctx := ctxlog.With(context.Background(), logger)

	database, err := db.Open(ctx, *dbPath)
	if err != nil {
		logger.Error("database error", "err", err)
		os.Exit(1)
	}
	defer func() {
		if err := database.Close(); err != nil {
			logger.Error("database close error", "err", err)
		}
	}()

	if err := db.Migrate(ctx, database); err != nil {
		logger.Error("migration error", "err", err)
		os.Exit(1)
	}

	plat, err := platforms.Create(*platformName)
	if err != nil {
		logger.Error("platform error", "err", err)
		os.Exit(1)
	}

	secretMgr := secrets.New(plat.Secrets())
	if err := secretMgr.Migrate(ctx); err != nil {
		logger.Error("secret migration error", "err", err)
		os.Exit(1)
	}

	if seeder, ok := plat.(platform.Seeder); ok {
		if err := seeder.SeedDefaults(ctx); err != nil {
			logger.Error("seed defaults error", "err", err)
			os.Exit(1)
		}
	}

	signingKey, err := secretMgr.SigningKey(ctx)
	if err != nil {
		logger.Error("signing key error", "err", err)
		os.Exit(1)
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		logger.Error("listen error", "err", err)
		os.Exit(1)
	}
	logger.Info("listening", "addr", ln.Addr())

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGHUP, os.Interrupt)

	for {
		srv := &http.Server{Handler: api.NewMux(ctx, plat, secretMgr, signingKey, dbqueries.New(database))}

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
