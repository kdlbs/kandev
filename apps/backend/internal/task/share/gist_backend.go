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
	client github.Client
}

// NewGistBackend returns a Backend that uses the given github.Client.
func NewGistBackend(client github.Client) *GistBackend {
	return &GistBackend{client: client}
}

// Name implements Backend.
func (b *GistBackend) Name() string { return BackendGitHubGist }

// Upload marshals the snapshot to JSON, builds the rendered share.html and
// a README, then creates a secret gist with all three. The returned URL is
// the rendered view served via gistpreview.github.io — that's the link
// users share. The gist itself is preserved in the database via the
// externalID so we can still delete it on revoke.
func (b *GistBackend) Upload(ctx context.Context, snap *Snapshot) (string, string, error) {
	body, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("marshal snapshot: %w", err)
	}
	if len(body) > GistMaxBytes {
		return "", "", fmt.Errorf("%w: %d bytes > %d", ErrSnapshotTooLarge, len(body), GistMaxBytes)
	}
	resp, err := b.client.CreateGist(ctx, github.CreateGistInput{
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
// for a given gist. We route through gistpreview.github.io, a static
// GitHub-Pages site that fetches the gist via the public API and renders
// an HTML file client-side. We use it instead of raw.githack.com so
// visitors don't hit the "One more step" anti-phishing interstitial.
//
// Critically: we append "/share.html" to the URL because gistpreview's
// file-picker logic picks the first iterated file unless one is named
// "index.html". Our gists contain README.md, share.html, snapshot.json —
// without the explicit suffix, gistpreview picks README.md (alphabetical
// first), renders it raw, and the page appears unstyled. The explicit
// filename pins it to share.html.
//
//	https://gist.github.com/<owner>/<id>  →  https://gistpreview.github.io/?<id>/share.html
//	https://gist.github.com/<id>          →  https://gistpreview.github.io/?<id>/share.html
//
// Returns "" if the URL doesn't look like a gist URL — callers fall back
// to whatever was stored.
func renderedURLForGist(gistHTMLURL string) string {
	id := gistIDFromGistHTMLURL(gistHTMLURL)
	if id == "" {
		return ""
	}
	return gistpreviewURL(id)
}

// gistpreviewURL builds the gistpreview.github.io URL for a gist id,
// explicitly pinning the file to share.html.
func gistpreviewURL(gistID string) string {
	return "https://gistpreview.github.io/?" + gistID + "/share.html"
}

// gistIDFromGistHTMLURL extracts the gist ID from a github.com gist URL.
// Returns "" for anything that doesn't match the expected shape.
func gistIDFromGistHTMLURL(gistHTMLURL string) string {
	const prefix = "https://gist.github.com/"
	if !strings.HasPrefix(gistHTMLURL, prefix) {
		return ""
	}
	rest := strings.Trim(strings.TrimPrefix(gistHTMLURL, prefix), "/")
	if rest == "" {
		return ""
	}
	// "<owner>/<id>" or "<id>" — the ID is always the last segment.
	parts := strings.Split(rest, "/")
	return parts[len(parts)-1]
}

// gistIDFromGithackURL pulls the gist ID out of a stored raw.githack URL
// (legacy rows that were written before the gistpreview switchover).
// Returns "" for anything that doesn't match.
func gistIDFromGithackURL(url string) string {
	const prefix = "https://gist.githack.com/"
	if !strings.HasPrefix(url, prefix) {
		return ""
	}
	parts := strings.Split(strings.TrimPrefix(url, prefix), "/")
	if len(parts) < 2 || parts[1] == "" {
		return ""
	}
	return parts[1]
}

// Delete removes the gist. A 404 from GitHub is returned to the caller as-is
// so the service can treat it as "already gone".
func (b *GistBackend) Delete(ctx context.Context, externalID string) error {
	if err := b.client.DeleteGist(ctx, externalID); err != nil {
		var apiErr *github.GitHubAPIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return err
		}
		return fmt.Errorf("delete gist %s: %w", externalID, err)
	}
	return nil
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
