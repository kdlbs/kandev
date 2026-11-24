package github

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode"
)

type repoCommitSHA struct {
	SHA string `json:"sha"`
}

func decodeRepoCommitSHA(raw []byte) (string, error) {
	var commits []repoCommitSHA
	if err := json.Unmarshal(raw, &commits); err != nil {
		return "", fmt.Errorf("decode repository commits: %w", err)
	}
	if len(commits) == 0 || commits[0].SHA == "" {
		return "", errors.New("no commit found for repository ref")
	}
	return commits[0].SHA, nil
}

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
