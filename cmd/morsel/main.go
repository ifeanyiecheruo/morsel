package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/ifeanyiecheruo/morsel/internal/cli"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
)

func main() {
	ctx := ctxlog.With(context.Background(), slog.Default())
	if err := cli.Execute(ctx, apiServerDockerfile); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
