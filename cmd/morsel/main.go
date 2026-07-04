package main

import (
	"context"
	"fmt"
	"os"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel/internal/cli"
)

func main() {
	if err := cli.Execute(context.Background(), apiServerDockerfile); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
