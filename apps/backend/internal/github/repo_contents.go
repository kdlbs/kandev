package github

import (
	"net/url"
	"strings"
	"unicode"
)

// repoContentsPath returns the URL-escaped path segment used by the
// `contents` REST/CLI endpoint for dir/path. Each path segment is escaped
// independently so slashes are preserved as path separators while special
// characters within a segment (spaces, '#', etc.) are safely encoded. Returns
// "" for the repository root ("", ".", or all-slash input).
func repoContentsPath(path string) string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" || trimmed == "." {
		return ""
	}
	segments := strings.Split(trimmed, "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	return strings.Join(segments, "/")
}

// repoContentsEndpoint builds the `/repos/{owner}/{repo}/contents[/{path}]`
// REST endpoint (with an optional `?ref=` query param) shared by
// PATClient.ListRepoDirectory / GetRepoFileContent.
func repoContentsEndpoint(owner, repo, path, ref string) string {
	endpoint := "/repos/" + owner + "/" + repo + "/contents"
	if p := repoContentsPath(path); p != "" {
		endpoint += "/" + p
	}
	if ref != "" {
		endpoint += "?ref=" + url.QueryEscape(ref)
	}
	return endpoint
}

// stripBase64Whitespace removes all whitespace (including the embedded
// newlines GitHub's contents API wraps base64 content with every 60
// characters) so the result can be fed directly to base64.StdEncoding.
func stripBase64Whitespace(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}
