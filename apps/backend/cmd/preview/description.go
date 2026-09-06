package main

import (
	"context"
	"flag"
	"fmt"
	"os"
)

func runUpdateDescription(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("update-description", flag.ContinueOnError)
	pr := fs.Int("pr", 0, "PR number (required)")
	sha := fs.String("sha", "", "commit SHA to display in the comment (required)")
	repo := fs.String("repo", envOr("GITHUB_REPOSITORY", ""), "owner/repo")
	previewURL := fs.String("url", "", "public preview URL (required)")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "preview update-description: %v\n", err)
		return 2
	}
	if *pr == 0 {
		fmt.Fprintln(os.Stderr, "preview update-description: --pr is required")
		return 2
	}
	if *sha == "" {
		fmt.Fprintln(os.Stderr, "preview update-description: --sha is required")
		return 2
	}
	if *repo == "" {
		fmt.Fprintln(os.Stderr, "preview update-description: --repo or GITHUB_REPOSITORY is required")
		return 2
	}
	if *previewURL == "" {
		fmt.Fprintln(os.Stderr, "preview update-description: --url is required")
		return 2
	}

	ghToken := os.Getenv("GH_TOKEN")
	if ghToken == "" {
		fmt.Fprintln(os.Stderr, "preview update-description: GH_TOKEN is required")
		return 2
	}

	section := buildDeploySection(*previewURL, *sha)
	if err := upsertDescriptionSection(ctx, ghToken, *repo, *pr, section); err != nil {
		fmt.Fprintf(os.Stderr, "preview update-description: update PR description: %v\n", err)
		return 1
	}
	return 0
}

func runRemoveDescription(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("remove-description", flag.ContinueOnError)
	pr := fs.Int("pr", 0, "PR number (required)")
	repo := fs.String("repo", envOr("GITHUB_REPOSITORY", ""), "owner/repo")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "preview remove-description: %v\n", err)
		return 2
	}
	if *pr == 0 {
		fmt.Fprintln(os.Stderr, "preview remove-description: --pr is required")
		return 2
	}
	if *repo == "" {
		fmt.Fprintln(os.Stderr, "preview remove-description: --repo or GITHUB_REPOSITORY is required")
		return 2
	}

	ghToken := os.Getenv("GH_TOKEN")
	if ghToken == "" {
		fmt.Fprintln(os.Stderr, "preview remove-description: GH_TOKEN is required")
		return 2
	}

	if err := removeDescriptionSection(ctx, ghToken, *repo, *pr); err != nil {
		fmt.Fprintf(os.Stderr, "preview remove-description: remove PR description section: %v\n", err)
		return 1
	}
	return 0
}
