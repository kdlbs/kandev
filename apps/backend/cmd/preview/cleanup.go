package main

import (
	"context"
	"flag"
	"fmt"
	"os"
)

type cleanupOptions struct {
	pr              int
	repo            string
	skipDescription bool
}

func parseCleanupOptions(args []string) (cleanupOptions, error) {
	fs := flag.NewFlagSet("cleanup", flag.ContinueOnError)
	pr := fs.Int("pr", 0, "PR number (required)")
	repo := fs.String("repo", envOr("GITHUB_REPOSITORY", ""), "owner/repo")
	skipDescription := fs.Bool("skip-description", false, "skip removing the PR description section")

	if err := fs.Parse(args); err != nil {
		return cleanupOptions{}, err
	}

	return cleanupOptions{
		pr:              *pr,
		repo:            *repo,
		skipDescription: *skipDescription,
	}, nil
}

func runCleanup(ctx context.Context, args []string) int {
	opts, err := parseCleanupOptions(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "preview cleanup: %v\n", err)
		return 2
	}
	if opts.pr == 0 {
		fmt.Fprintln(os.Stderr, "preview cleanup: --pr is required")
		return 2
	}
	if opts.repo == "" {
		fmt.Fprintln(os.Stderr, "preview cleanup: --repo or GITHUB_REPOSITORY is required")
		return 2
	}

	spritesToken := os.Getenv("SPRITES_API_TOKEN")
	if spritesToken == "" {
		fmt.Fprintln(os.Stderr, "preview cleanup: SPRITES_API_TOKEN is required")
		return 2
	}
	ghToken := os.Getenv("GH_TOKEN")
	if ghToken == "" && !opts.skipDescription {
		fmt.Fprintln(os.Stderr, "preview cleanup: GH_TOKEN is required")
		return 2
	}

	spriteName := fmt.Sprintf("kandev-pr-%d", opts.pr)

	client := newSpriteClient(spritesToken)
	defer func() { _ = client.Close() }()

	fmt.Fprintf(os.Stderr, "destroying sprite %s...\n", spriteName)
	_, destroyErr := destroySprite(ctx, client, spriteName)
	if destroyErr != nil {
		fmt.Fprintf(os.Stderr, "preview cleanup: destroy sprite: %v\n", destroyErr)
		// Continue to update the PR description even if destroy failed.
	}

	if opts.skipDescription {
		if destroyErr != nil {
			return 1
		}
		return 0
	}

	if err := removeDescriptionSection(ctx, ghToken, opts.repo, opts.pr); err != nil {
		fmt.Fprintf(os.Stderr, "preview cleanup: remove PR description section: %v\n", err)
		return 1
	}

	if destroyErr != nil {
		return 1
	}
	return 0
}
