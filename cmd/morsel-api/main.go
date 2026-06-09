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
	"github.com/ifeanyiecheruo/morsel/internal/platforms"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	platformName := flag.String("platform", "", "platform implementation (local|gcp)")
	flag.Parse()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		slog.Error("listen error", "err", err)
		os.Exit(1)
	}
	slog.Info("listening", "addr", ln.Addr())

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGHUP, os.Interrupt)

	for {
		plat, err := platforms.Create(*platformName)
		if err != nil {
			slog.Error("platform error", "err", err)
			os.Exit(1)
		}

		srv := &http.Server{Handler: api.NewMux(plat)}

		go func() {
			if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
				slog.Error("server error", "err", err)
				os.Exit(1)
			}
		}()

		sig := <-quit
		slog.Info("signal received", "signal", sig)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := srv.Shutdown(ctx); err != nil {
			slog.Error("shutdown error", "err", err)
		}
		cancel()

		if sig == syscall.SIGHUP {
			slog.Info("reloading")
			continue
		}

		ln.Close()
		slog.Info("shutdown complete")
		return
	}
}
