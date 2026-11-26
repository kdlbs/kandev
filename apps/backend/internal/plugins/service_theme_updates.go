package plugins

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/kandev/kandev/internal/plugins/marketplace"
	"github.com/kandev/kandev/internal/plugins/pkgtar"
	"github.com/kandev/kandev/internal/plugins/store"
)

type themeRelease struct{ URL, ID, Version, Digest string }

func (s *Service) downloadTheme(ctx context.Context, request themeRelease) ([]byte, error) {
	if err := validateInstallURL(request.URL); err != nil {
		return nil, err
	}
	digest, err := hex.DecodeString(request.Digest)
	if err != nil || len(digest) != sha256.Size || request.ID == "" || request.Version == "" {
		return nil, fmt.Errorf("theme_identity: release id, version and sha256 required")
	}
	client := *s.httpClient
	client.Timeout = downloadTimeout
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("theme_download: too many redirects")
		}
		return validateInstallURL(req.URL.String())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, request.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("theme_download: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, pkgtar.MaxThemePackageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > pkgtar.MaxThemePackageBytes {
		return nil, fmt.Errorf("theme_size: package limit exceeded")
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != strings.ToLower(request.Digest) {
		return nil, fmt.Errorf("theme_digest: release checksum mismatch")
	}
	return data, nil
}

func (s *Service) installMarketplaceUpdate(ctx context.Context, entry marketplace.CatalogEntry) (*store.Record, error) {
	installed, ok := s.registry.Get(entry.ID)
	installedTheme := ok && installed.IsThemeDistribution()
	if !installedTheme && entry.Kind != "theme" {
		return s.InstallFromURL(ctx, entry.PackageURL)
	}
	if entry.Kind != "theme" || (ok && !installedTheme) {
		return nil, fmt.Errorf("theme_profile: selected release disagrees with installed package kind")
	}
	data, err := s.downloadTheme(ctx, themeRelease{URL: entry.PackageURL, ID: entry.ID, Version: entry.Version, Digest: entry.PackageSHA256})
	if err != nil {
		return nil, err
	}
	// Validate the selected release in isolation before admitting these same bytes.
	root, err := os.MkdirTemp("", "kandev-theme-update-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(root) }()
	verified, err := pkgtar.Install(bytes.NewReader(data), root)
	if err != nil {
		return nil, err
	}
	if !verified.Manifest.IsThemeDistribution() {
		return nil, fmt.Errorf("theme_profile: theme package required")
	}
	if verified.Manifest.ID != entry.ID || verified.Manifest.Version != entry.Version {
		return nil, fmt.Errorf("theme_identity: archive does not match selected release")
	}
	return s.Install(ctx, bytes.NewReader(data))
}
