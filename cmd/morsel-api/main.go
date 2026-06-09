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
	"github.com/ifeanyiecheruo/morsel/internal/platforms"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	platformName := flag.String("platform", "", "platform implementation (local|gcp)")
	dbPath := flag.String("db", "morsel.db", "SQLite database path")
	flag.Parse()

	logger := slog.Default()
	ctx := ctxlog.With(context.Background(), logger)

	database, err := db.Open(*dbPath)
	if err != nil {
		logger.Error("database error", "err", err)
		os.Exit(1)
	}
	defer database.Close()

	if err := db.Migrate(ctx, database); err != nil {
		logger.Error("migration error", "err", err)
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
		plat, err := platforms.Create(*platformName)
		if err != nil {
			logger.Error("platform error", "err", err)
			os.Exit(1)
		}

		srv := &http.Server{Handler: api.NewMux(plat)}

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

		ln.Close()
		logger.Info("shutdown complete")
		return
	}
}
