package share

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/kandev/kandev/internal/github"
)

// GistMaxBytes is a conservative cap on a single snapshot.json file. GitHub
// caps individual gist files at 100 MiB but 10 MiB is plenty for any
// reasonable conversation and avoids accidentally pushing huge payloads.
const GistMaxBytes = 10 * 1024 * 1024

// GistBackend uploads snapshots to GitHub Gists on the authenticated user's
// account. Created gists are always secret (unlisted); anyone with the URL
// can view them but they are not crawled.
type GistBackend struct {
	client   github.Client
	resolver GitHubClientResolver
}

// GitHubClientResolver resolves the automation principal selected by a
// workspace. Implementations must fail closed for missing workspace IDs.
type GitHubClientResolver interface {
	ResolveGitHubAutomationClient(context.Context, string) (github.Client, error)
}

// NewGistBackend returns a Backend that uses the given github.Client.
func NewGistBackend(client github.Client) *GistBackend {
	return &GistBackend{client: client}
}

// NewWorkspaceGistBackend returns a Gist backend that resolves a client for
// each owning workspace instead of retaining an installation-global client.
func NewWorkspaceGistBackend(resolver GitHubClientResolver) *GistBackend {
	return &GistBackend{resolver: resolver}
}

// Name implements Backend.
func (b *GistBackend) Name() string { return BackendGitHubGist }

// Upload marshals the snapshot to JSON, builds the rendered share.html and
// a README, then creates a secret gist with all three. The returned URL is
// the rendered view served via gist.githack.com — that's the link users
// share. The gist itself is preserved in the database via the externalID
// so we can still delete it on revoke.
func (b *GistBackend) Upload(ctx context.Context, workspaceID string, snap *Snapshot) (string, string, error) {
	body, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("marshal snapshot: %w", err)
	}
	if len(body) > GistMaxBytes {
		return "", "", fmt.Errorf("%w: %d bytes > %d", ErrSnapshotTooLarge, len(body), GistMaxBytes)
	}
	client, err := b.clientFor(ctx, workspaceID)
	if err != nil {
		return "", "", err
	}
	resp, err := client.CreateGist(ctx, github.CreateGistInput{
		Description: gistDescription(snap),
		Public:      false,
		Files: map[string]github.GistFile{
			// share.html sorts first alphabetically in the gist file list,
			// which puts the rendered view at the top of the gist page too.
			"share.html":    {Content: BuildShareHTML(snap)},
			"snapshot.json": {Content: string(body)},
			// README is built with empty renderedURL — the user's primary
			// link goes to the rendered view; the README is a fallback for
			// folks who land on the gist directly.
			"README.md": {Content: BuildGistREADME(snap, "")},
		},
	})
	if err != nil {
		return "", "", fmt.Errorf("create gist: %w", err)
	}
	rendered := renderedURLForGist(resp.HTMLURL)
	if rendered == "" {
		// Fall back to the raw gist URL if we couldn't parse a gist ID —
		// not pretty, but at least the user has a working link.
		rendered = resp.HTMLURL
	}
	return resp.ID, rendered, nil
}

// renderedURLForGist returns the public rendering URL we hand to the user
// for a given gist. We route through gist.githack.com, a Cloudflare-backed
// CDN that proxies the gist's raw file content directly.
//
// Why githack and not gistpreview.github.io: gistpreview fetches gists
// through the GitHub gists API, which serves `content: ""` for any file
// past GitHub's per-response content budget (~1 MB combined across all
// files). For non-trivial sessions, share.html / snapshot.json land past
// that budget, and gistpreview renders a blank page. githack proxies the
// raw file directly, so multi-megabyte share.html renders correctly.
//
// Trade-off: githack shows a one-time anti-phishing interstitial the first
// time a user visits any githack URL — accepted as the price of fidelity
// for big tasks.
//
// We append "/raw/share.html" to pin the styled HTML view; without an
// explicit filename githack can't pick the right file.
//
//	https://gist.github.com/<owner>/<id>  →  https://gist.githack.com/<owner>/<id>/raw/share.html
//
// Returns "" if the URL is missing the owner segment (anonymous gist) —
// callers fall back to whatever was stored.
func renderedURLForGist(gistHTMLURL string) string {
	owner, id := ownerAndIDFromGistHTMLURL(gistHTMLURL)
	if owner == "" || id == "" {
		return ""
	}
	return githackURL(owner, id)
}

// githackURL builds the gist.githack.com raw-render URL for a gist,
// explicitly pinning the file to share.html so the styled view loads.
func githackURL(owner, id string) string {
	return "https://gist.githack.com/" + owner + "/" + id + "/raw/share.html"
}

// ownerAndIDFromGistHTMLURL parses a github.com gist URL of the form
// `https://gist.github.com/<owner>/<id>`. Anonymous gists (no owner
// segment) return ("", ""); githack needs both to address the file.
func ownerAndIDFromGistHTMLURL(gistHTMLURL string) (string, string) {
	const prefix = "https://gist.github.com/"
	if !strings.HasPrefix(gistHTMLURL, prefix) {
		return "", ""
	}
	rest := strings.Trim(strings.TrimPrefix(gistHTMLURL, prefix), "/")
	if rest == "" {
		return "", ""
	}
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", ""
	}
	return parts[0], parts[1]
}

// ownerAndIDFromGithackURL extracts (owner, id) from a stored githack URL
// of the form `https://gist.githack.com/<owner>/<id>/raw/...`. Used by
// displayURL to re-pin /raw/share.html on rows whose stored URL targets
// a different file. Returns ("", "") on anything that doesn't match.
func ownerAndIDFromGithackURL(url string) (string, string) {
	const prefix = "https://gist.githack.com/"
	if !strings.HasPrefix(url, prefix) {
		return "", ""
	}
	parts := strings.Split(strings.TrimPrefix(url, prefix), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", ""
	}
	return parts[0], parts[1]
}

// Delete removes the gist. A 404 from GitHub is returned to the caller as-is
// so the service can treat it as "already gone".
func (b *GistBackend) Delete(ctx context.Context, workspaceID, externalID string) error {
	client, err := b.clientFor(ctx, workspaceID)
	if err != nil {
		return err
	}
	if err := client.DeleteGist(ctx, externalID); err != nil {
		var apiErr *github.GitHubAPIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return err
		}
		return fmt.Errorf("delete gist %s: %w", externalID, err)
	}
	return nil
}

// CheckAccess verifies the resolved workspace client before share creation.
func (b *GistBackend) CheckAccess(ctx context.Context, workspaceID string) error {
	client, err := b.clientFor(ctx, workspaceID)
	if err != nil {
		return err
	}
	authenticated, err := client.IsAuthenticated(ctx)
	if err != nil {
		return err
	}
	if !authenticated {
		return errors.New("connect a GitHub account to share tasks publicly")
	}
	return nil
}

func (b *GistBackend) clientFor(ctx context.Context, workspaceID string) (github.Client, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, ErrWorkspaceRequired
	}
	if b != nil && b.resolver != nil {
		client, err := b.resolver.ResolveGitHubAutomationClient(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		if client == nil {
			return nil, errors.New("github client resolver returned no client")
		}
		return client, nil
	}
	if b == nil || b.client == nil {
		return nil, errors.New("github client is not configured")
	}
	return b.client, nil
}

func gistDescription(snap *Snapshot) string {
	if snap == nil || snap.Task.Title == "" {
		return "kandev task share"
	}
	return "kandev share: " + snap.Task.Title
}

func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
