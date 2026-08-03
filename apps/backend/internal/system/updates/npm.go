package updates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const DefaultNPMRegistryURL = "https://registry.npmjs.org/kandev"
const maxNPMPackageResponseBytes = 8 << 20

type npmPackagePayload struct {
	DistTags map[string]string          `json:"dist-tags"`
	Versions map[string]json.RawMessage `json:"versions"`
}

type npmVersionRecord struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func FetchLatestNightlyFrom(ctx context.Context, client *http.Client, registryURL string) (string, string, error) {
	if client == nil {
		client = &http.Client{Timeout: defaultClientTimeout}
	}
	if registryURL == "" {
		registryURL = DefaultNPMRegistryURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, registryURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("build npm request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.npm.install-v1+json")
	req.Header.Set("User-Agent", "kandev-updates-poller")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("npm request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", "", fmt.Errorf("npm registry returned status %d", resp.StatusCode)
	}

	payload, err := decodeNPMPackagePayload(resp.Body)
	if err != nil {
		return "", "", err
	}
	version := payload.DistTags[string(ChannelNightly)]
	if version == "" {
		return "", "", errors.New("npm response missing nightly dist-tag")
	}
	// npm package versions and dist-tag targets use canonical bare SemVer.
	// Internal update tags add v only after this registry integrity check.
	if !isCanonicalNPMNightlyVersion(version) {
		return "", "", fmt.Errorf("npm response has invalid nightly version %q", version)
	}
	record, ok := payload.Versions[version]
	if !ok || len(record) == 0 || string(record) == "null" {
		return "", "", fmt.Errorf("npm response missing exact version %q", version)
	}
	if !isExactNPMVersionRecord(record, version) {
		return "", "", fmt.Errorf("npm response has invalid exact version record %q", version)
	}

	return "v" + version, "https://www.npmjs.com/package/kandev/v/" + url.PathEscape(version), nil
}

func isExactNPMVersionRecord(record json.RawMessage, version string) bool {
	var exact npmVersionRecord
	return json.Unmarshal(record, &exact) == nil &&
		exact.Name == "kandev" && exact.Version == version
}

func isCanonicalNPMNightlyVersion(version string) bool {
	return !strings.HasPrefix(version, "v") && isNightlyVersion(version)
}

func decodeNPMPackagePayload(reader io.Reader) (npmPackagePayload, error) {
	body, err := io.ReadAll(io.LimitReader(reader, maxNPMPackageResponseBytes+1))
	if err != nil {
		return npmPackagePayload{}, fmt.Errorf("read npm response: %w", err)
	}
	if len(body) > maxNPMPackageResponseBytes {
		return npmPackagePayload{}, fmt.Errorf("npm response exceeds %d bytes", maxNPMPackageResponseBytes)
	}
	var payload npmPackagePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return npmPackagePayload{}, fmt.Errorf("decode npm response: %w", err)
	}
	return payload, nil
}
