package controller

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

func (c *Controller) resolveGitHubRuntimeRelease(ctx context.Context, repository string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+repository+"/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "kandev-runtime-updates")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return "", errors.New("runtime release source unavailable")
	}
	var release struct {
		TagName    string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1024*1024)).Decode(&release); err != nil {
		return "", err
	}
	if release.Draft || release.Prerelease {
		return "", errors.New("runtime release is not stable")
	}
	return validateRuntimeUpdateLatest(strings.TrimPrefix(release.TagName, "v"), nil)
}
