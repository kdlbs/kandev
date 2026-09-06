package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func runPackage(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("package", flag.ContinueOnError)
	artifact := fs.String("artifact", "", "path for the preview bundle (required)")
	skipWebInstall := fs.Bool("skip-web-install", false, "skip pnpm install (CI already ran it)")
	skipWebBuild := fs.Bool("skip-web-build", false, "package existing web dist without running fork build scripts")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "preview package: %v\n", err)
		return 2
	}
	if *artifact == "" {
		fmt.Fprintln(os.Stderr, "preview package: --artifact is required")
		return 2
	}

	binDir, err := os.MkdirTemp("", "kandev-preview-bin-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "preview package: mktemp: %v\n", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(binDir) }()

	if err := os.MkdirAll(filepath.Dir(*artifact), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "preview package: create artifact directory: %v\n", err)
		return 1
	}
	if err := buildPreviewArtifact(ctx, binDir, *artifact, *skipWebInstall, *skipWebBuild); err != nil {
		fmt.Fprintf(os.Stderr, "preview package: %v\n", err)
		return 1
	}
	return 0
}
