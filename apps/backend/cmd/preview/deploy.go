package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kandev/kandev/internal/common/ports"
	sprites "github.com/superfly/sprites-go"
)

func runDeploy(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("deploy", flag.ContinueOnError)
	pr := fs.Int("pr", 0, "PR number (required)")
	sha := fs.String("sha", "", "commit SHA to display in the comment")
	repo := fs.String("repo", envOr("GITHUB_REPOSITORY", ""), "owner/repo")
	port := fs.Int("port", ports.Backend, "kandev backend port exposed by the sprite")
	skipWebInstall := fs.Bool("skip-web-install", false, "skip pnpm install (CI already ran it)")

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

	previewURL, err := deployArtifacts(ctx, binDir, tarPath, spritesToken, spriteName, *port, *skipWebInstall)
	if err != nil {
		fmt.Fprintf(os.Stderr, "preview deploy: %v\n", err)
		return 1
	}

	fmt.Printf("preview URL: %s\n", previewURL)

	section := buildDeploySection(previewURL, *sha)
	if err := upsertDescriptionSection(ctx, ghToken, *repo, *pr, section); err != nil {
		fmt.Fprintf(os.Stderr, "preview deploy: update PR description: %v\n", err)
		return 1
	}

	return 0
}

// deployArtifacts builds the bundle, deploys it to the sprite, and returns the public URL.
// Using a single client for the full flow avoids redundant auth round-trips.
func deployArtifacts(ctx context.Context, binDir, tarPath, spritesToken, spriteName string, port int, skipWebInstall bool) (string, error) {
	fmt.Fprintln(os.Stderr, "building linux/amd64 binaries...")
	if err := buildLinuxBinaries(ctx, binDir); err != nil {
		return "", fmt.Errorf("build binaries: %w", err)
	}

	fmt.Fprintln(os.Stderr, "building web frontend...")
	if err := buildWeb(ctx, skipWebInstall); err != nil {
		return "", fmt.Errorf("build web: %w", err)
	}

	fmt.Fprintln(os.Stderr, "packaging bundle...")
	if err := packageBundle(binDir, tarPath); err != nil {
		return "", fmt.Errorf("package bundle: %w", err)
	}

	client := newSpriteClient(spritesToken)
	defer func() { _ = client.Close() }()

	fmt.Fprintf(os.Stderr, "getting or creating sprite %s...\n", spriteName)
	sprite, err := getOrCreateSprite(ctx, client, spriteName)
	if err != nil {
		return "", fmt.Errorf("get/create sprite: %w", err)
	}

	fmt.Fprintln(os.Stderr, "uploading bundle...")
	if err := uploadBundle(ctx, sprite, tarPath); err != nil {
		return "", fmt.Errorf("upload bundle: %w", err)
	}

	fmt.Fprintln(os.Stderr, "extracting and configuring...")
	if err := extractBundle(ctx, sprite); err != nil {
		return "", fmt.Errorf("extract bundle: %w", err)
	}

	fmt.Fprintln(os.Stderr, "deploying kandev service...")
	if err := deployService(ctx, sprite, port); err != nil {
		return "", fmt.Errorf("deploy service: %w", err)
	}

	// Enable the public URL before the health check so that hitting the URL
	// also triggers any lazy service startup on Sprites' side.
	previewURL, err := enablePublicURL(ctx, client, spriteName)
	if err != nil {
		return "", fmt.Errorf("enable public URL: %w", err)
	}

	fmt.Fprintf(os.Stderr, "waiting for kandev to be healthy at %s...\n", previewURL)
	if err := waitForKandev(ctx, sprite, previewURL); err != nil {
		return "", fmt.Errorf("health check: %w", err)
	}

	return previewURL, nil
}

// enablePublicURL sets the sprite's URL to public mode and returns the URL.
// Accepts the already-open client from deployArtifacts to avoid a second auth handshake.
func enablePublicURL(ctx context.Context, client *sprites.Client, spriteName string) (string, error) {
	updateCtx, cancel := context.WithTimeout(ctx, spriteStepTimeout)
	defer cancel()

	if err := client.UpdateURLSettings(updateCtx, spriteName, &sprites.URLSettings{Auth: "public"}); err != nil {
		return "", fmt.Errorf("update URL settings: %w", err)
	}

	getCtx, getCancel := context.WithTimeout(ctx, spriteStepTimeout)
	defer getCancel()

	sprite, err := client.GetSprite(getCtx, spriteName)
	if err != nil {
		return "", fmt.Errorf("get sprite URL: %w", err)
	}
	if sprite.URL == "" {
		return "", fmt.Errorf("sprite %s has no URL assigned yet", spriteName)
	}
	return sprite.URL, nil
}
