package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
)

// runServer binds addr, then loops: start the server returned by newServer,
// wait for a signal, drain active connections, then either reload (SIGHUP) or
// exit (SIGTERM / Interrupt). On SIGHUP the listener is re-bound so a fresh
// server instance can serve without holding a closed listener.
//
// shutdownTimeout of 0 uses a 30-second default.
func runServer(ctx context.Context, addr string, shutdownTimeout time.Duration, newServer func() *http.Server) {
	logger := ctxlog.From(ctx)
	if shutdownTimeout <= 0 {
		shutdownTimeout = 30 * time.Second
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGHUP, os.Interrupt)
	defer signal.Stop(quit)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Error("listen", "addr", addr, "err", err)
		os.Exit(1)
	}
	logger.Info("listening", "addr", ln.Addr())

	for {
		srv := newServer()
		go func() {
			if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("server error", "err", err)
				os.Exit(1)
			}
		}()

		sig := <-quit
		logger.Info("signal received", "signal", sig)

		shutCtx, cancel := context.WithTimeout(ctx, shutdownTimeout)
		if err := srv.Shutdown(shutCtx); err != nil {
			logger.Error("shutdown error", "err", err)
		}
		cancel()

		if sig == syscall.SIGHUP {
			logger.Info("reloading")
			// Shutdown closed the listener; re-bind before the next iteration.
			ln, err = net.Listen("tcp", addr)
			if err != nil {
				logger.Error("listen after reload", "addr", addr, "err", err)
				os.Exit(1)
			}
			continue
		}

		logger.Info("shutdown complete")
		return
	}
}
