package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kandev/kandev/internal/common/ports"
)

func runDeploy(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("deploy", flag.ContinueOnError)
	pr := fs.Int("pr", 0, "PR number (required)")
	sha := fs.String("sha", "", "commit SHA to display in the comment")
	repo := fs.String("repo", envOr("GITHUB_REPOSITORY", ""), "owner/repo")
	port := fs.Int("port", ports.Backend, "kandev backend port exposed by the sprite")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "preview deploy: %v\n", err)
		return 2
	}
	if *pr == 0 {
		fmt.Fprintln(os.Stderr, "preview deploy: --pr is required")
		return 2
	}
	if *repo == "" {
		fmt.Fprintln(os.Stderr, "preview deploy: --repo or GITHUB_REPOSITORY is required")
		return 2
	}

	spritesToken := os.Getenv("SPRITES_API_TOKEN")
	if spritesToken == "" {
		fmt.Fprintln(os.Stderr, "preview deploy: SPRITES_API_TOKEN is required")
		return 2
	}
	ghToken := os.Getenv("GH_TOKEN")
	if ghToken == "" {
		fmt.Fprintln(os.Stderr, "preview deploy: GH_TOKEN is required")
		return 2
	}

	spriteName := fmt.Sprintf("kandev-pr-%d", *pr)

	tmpDir, err := os.MkdirTemp("", "kandev-preview-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "preview deploy: mktemp: %v\n", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	binDir := filepath.Join(tmpDir, "bin")
	tarPath := filepath.Join(tmpDir, "kandev-preview.tar.gz")

	if err := deployArtifacts(ctx, binDir, tarPath, spritesToken, spriteName, *port); err != nil {
		fmt.Fprintf(os.Stderr, "preview deploy: %v\n", err)
		return 1
	}

	previewURL, err := enablePublicURL(ctx, spritesToken, spriteName, *port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "preview deploy: enable public URL: %v\n", err)
		return 1
	}

	fmt.Printf("preview URL: %s\n", previewURL)

	body := buildDeployComment(previewURL, *sha)
	if err := upsertComment(ctx, ghToken, *repo, *pr, body); err != nil {
		fmt.Fprintf(os.Stderr, "preview deploy: post comment: %v\n", err)
		return 1
	}

	return 0
}

func deployArtifacts(ctx context.Context, binDir, tarPath, spritesToken, spriteName string, port int) error {
	fmt.Println("building linux/amd64 binaries...")
	if err := buildLinuxBinaries(ctx, binDir); err != nil {
		return fmt.Errorf("build binaries: %w", err)
	}

	fmt.Println("building web frontend...")
	if err := buildWeb(ctx); err != nil {
		return fmt.Errorf("build web: %w", err)
	}

	fmt.Println("packaging bundle...")
	if err := packageBundle(binDir, tarPath); err != nil {
		return fmt.Errorf("package bundle: %w", err)
	}

	client := newSpriteClient(spritesToken)
	defer func() { _ = client.Close() }()

	fmt.Printf("getting or creating sprite %s...\n", spriteName)
	sprite, err := getOrCreateSprite(ctx, client, spriteName)
	if err != nil {
		return fmt.Errorf("get/create sprite: %w", err)
	}

	fmt.Println("uploading bundle...")
	if err := uploadBundle(ctx, sprite, tarPath); err != nil {
		return fmt.Errorf("upload bundle: %w", err)
	}

	fmt.Println("extracting and configuring...")
	if err := extractBundle(ctx, sprite); err != nil {
		return fmt.Errorf("extract bundle: %w", err)
	}

	fmt.Println("deploying kandev service...")
	if err := deployService(ctx, sprite, port); err != nil {
		return fmt.Errorf("deploy service: %w", err)
	}

	return nil
}
