package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"
)

func runCleanup(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("cleanup", flag.ContinueOnError)
	pr := fs.Int("pr", 0, "PR number (required)")
	repo := fs.String("repo", envOr("GITHUB_REPOSITORY", ""), "owner/repo")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "preview cleanup: %v\n", err)
		return 2
	}
	if *pr == 0 {
		fmt.Fprintln(os.Stderr, "preview cleanup: --pr is required")
		return 2
	}
	if *repo == "" {
		fmt.Fprintln(os.Stderr, "preview cleanup: --repo or GITHUB_REPOSITORY is required")
		return 2
	}

	spritesToken := os.Getenv("SPRITES_API_TOKEN")
	if spritesToken == "" {
		fmt.Fprintln(os.Stderr, "preview cleanup: SPRITES_API_TOKEN is required")
		return 2
	}
	ghToken := os.Getenv("GH_TOKEN")
	if ghToken == "" {
		fmt.Fprintln(os.Stderr, "preview cleanup: GH_TOKEN is required")
		return 2
	}

	spriteName := fmt.Sprintf("kandev-pr-%d", *pr)

	client := newSpriteClient(spritesToken)
	defer func() { _ = client.Close() }()

	fmt.Printf("destroying sprite %s...\n", spriteName)
	createdAt, err := destroySprite(ctx, client, spriteName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "preview cleanup: destroy sprite: %v\n", err)
		// Don't fail — sprite may already be gone; still try to update comment.
	}

	runtime := computeRuntime(createdAt)
	body := buildCleanupComment(runtime)
	if err := upsertComment(ctx, ghToken, *repo, *pr, body); err != nil {
		fmt.Fprintf(os.Stderr, "preview cleanup: update comment: %v\n", err)
		return 1
	}

	return 0
}

func computeRuntime(createdAt time.Time) time.Duration {
	if createdAt.IsZero() {
		return 0
	}
	return time.Since(createdAt).Round(time.Minute)
}
