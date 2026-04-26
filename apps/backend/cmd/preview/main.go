// Command preview deploys and manages PR preview environments on Sprites.dev.
//
// Usage:
//
//	preview deploy  --pr N --sha S [--repo owner/repo] [--port ports.Backend]
//	preview cleanup --pr N [--repo owner/repo]
//
// Required environment variables:
//
//	SPRITES_API_TOKEN  Sprites.dev API token
//	GH_TOKEN           GitHub token for posting PR comments
//	GITHUB_REPOSITORY  Repository in owner/repo format
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() { os.Exit(run()) }

func run() int {
	if len(os.Args) < 2 {
		usage()
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	switch os.Args[1] {
	case "deploy":
		return runDeploy(ctx, os.Args[2:])
	case "cleanup":
		return runCleanup(ctx, os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "preview: unknown subcommand %q\n\n", os.Args[1])
		usage()
		return 2
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `preview — deploy and manage PR preview environments on Sprites.dev

Usage:
  preview deploy  --pr N --sha S [--repo owner/repo] [--port ports.Backend]
  preview cleanup --pr N [--repo owner/repo]

Environment variables:
  SPRITES_API_TOKEN  Sprites.dev API token (required)
  GH_TOKEN           GitHub token for posting PR comments (required)
  GITHUB_REPOSITORY  GitHub repository in owner/repo format (required)

Local usage:
  cd apps/backend
  SPRITES_API_TOKEN=xxx GH_TOKEN=xxx GITHUB_REPOSITORY=owner/repo \
    go run ./cmd/preview deploy --pr 123 --sha abc1234`)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
