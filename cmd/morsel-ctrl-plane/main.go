package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "api":
		runAPI(os.Args[2:])
	case "svc":
		if len(os.Args) < 3 {
			usage()
			os.Exit(2)
		}
		switch os.Args[2] {
		case "queue":
			runQueueService(os.Args[3:])
		case "blob":
			fmt.Fprintln(os.Stderr, "morsel-ctrl-plane svc blob: not implemented")
			os.Exit(1)
		default:
			usage()
			os.Exit(2)
		}
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: morsel-ctrl-plane <command>

Commands:
  api         Run the control-plane REST API server
  svc queue   Run the queue service
  svc blob    Run the blob service (not yet implemented)

Run "morsel-ctrl-plane <command> -help" for command-specific flags.`)
}
