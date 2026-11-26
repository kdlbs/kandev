package plugins

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/plugins/marketplace"
	"github.com/kandev/kandev/internal/plugins/pkgtar/pkgtartest"
)

type themeUpdateTransport func(*http.Request) (*http.Response, error)

func (f themeUpdateTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func themeUpdateArchive(t *testing.T, id, version string) []byte {
	t.Helper()
	var archive bytes.Buffer
	files := map[string][]byte{"manifest.yaml": []byte(fmt.Sprintf(`id: %s
api_version: 2
version: %s
display_name: Sample themes
distribution: {schema_version: 1, kind: theme}
themes: [{key: dark, label: Dark, base: dark, path: dark.json}]
`, id, version)), "dark.json": []byte(`{"schema_version":1,"ui":{"background":"#272822"}}`)}
	if err := pkgtartest.WritePackage(&archive, files); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}

// @covers AC-PLUGINS-THEMES-001.2, AC-PLUGINS-THEMES-003.1, AC-PLUGINS-THEMES-003.3
func TestThemeMarketplaceUpdateAdmission(t *testing.T) {
	for _, scenario := range []string{"valid", "executable", "executable-same-id", "other-id", "other-version", "digest", "missing-digest", "missing-version", "listing-kind", "missing-kind"} {
		t.Run(scenario, func(t *testing.T) {
			svc, _, rt := newTestService(t)
			rec, err := svc.Install(context.Background(), testThemeArchive(t))
			if err != nil {
				t.Fatal(err)
			}
			if _, err = svc.SetPluginAutoUpdate(rec.ID, bptr(true)); err != nil {
				t.Fatal(err)
			}
			before, _ := svc.registry.Get(rec.ID)
			inventory := svc.ThemeInventory()
			payload := themeUpdateArchive(t, rec.ID, "2.0.0")
			switch scenario {
			case "executable":
				payload = testPackage(t, "unexpected-runtime", "2.0.0", true).Bytes()
			case "executable-same-id":
				payload = testPackage(t, rec.ID, "2.0.0", true).Bytes()
			case "other-id":
				payload = themeUpdateArchive(t, "another-theme", "2.0.0")
			case "other-version":
				payload = themeUpdateArchive(t, rec.ID, "3.0.0")
			}
			sum := sha256.Sum256(payload)
			entry := marketplace.CatalogEntry{IndexEntry: marketplace.IndexEntry{ID: rec.ID, Kind: "theme", Version: "2.0.0", PackageURL: "https://example.test/theme.tar.gz", PackageSHA256: hex.EncodeToString(sum[:])}, InstallState: marketplace.StateUpdateAvailable, InstalledVersion: "1.0.0"}
			switch scenario {
			case "digest":
				entry.PackageSHA256 = string(bytes.Repeat([]byte("0"), 64))
			case "missing-digest":
				entry.PackageSHA256 = ""
			case "missing-version":
				entry.Version = ""
			case "missing-kind":
				entry.Kind = ""
			case "listing-kind":
				entry.Kind = "plugin"
			}
			svc.httpClient = &http.Client{Transport: themeUpdateTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(payload)), Header: make(http.Header)}, nil
			})}
			outcome := svc.applyAutoUpdates(context.Background(), []marketplace.CatalogEntry{entry})
			if len(rt.startCalls) != 0 || len(rt.stopCalls) != 0 {
				t.Fatalf("runtime admission: starts %v stops %v", rt.startCalls, rt.stopCalls)
			}
			after, _ := svc.registry.Get(rec.ID)
			if scenario == "valid" {
				if len(outcome.Updated) != 1 || len(outcome.Failed) != 0 || after.Version != "2.0.0" {
					t.Fatalf("valid release rejected: %+v, %+v", outcome, after)
				}
				if len(svc.ThemeInventory().Themes) != 1 {
					t.Fatal("updated inventory unavailable")
				}
				return
			}
			if len(outcome.Failed) != 1 || len(outcome.Updated) != 0 {
				t.Fatalf("invalid release admitted: %+v", outcome)
			}
			if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(inventory, svc.ThemeInventory()) {
				t.Fatal("failed update changed installed theme")
			}
			if len(svc.List()) != 1 {
				t.Fatal("unrelated package installed")
			}
			data, err := os.ReadFile(filepath.Join(after.InstallPath, "dark.json"))
			if err != nil || !bytes.Contains(data, []byte("#272822")) {
				t.Fatalf("old theme unavailable: %s %v", data, err)
			}
			versions, err := os.ReadDir(filepath.Dir(after.InstallPath))
			if err != nil || len(versions) != 1 {
				t.Fatalf("failed update published files: %v %v", versions, err)
			}
		})
	}
}

func TestThemeMarketplaceUpdateMissingIdentity(t *testing.T) {
	for _, field := range []string{"id", "version", "digest"} {
		t.Run(field, func(t *testing.T) {
			svc, _, rt := newTestService(t)
			requested := false
			svc.httpClient = &http.Client{Transport: themeUpdateTransport(func(*http.Request) (*http.Response, error) {
				requested = true
				return nil, fmt.Errorf("unexpected download")
			})}
			release := themeRelease{URL: "https://example.test/theme.tar.gz", ID: "sample-themes", Version: "2.0.0", Digest: strings.Repeat("a", 64)}
			switch field {
			case "id":
				release.ID = ""
			case "version":
				release.Version = ""
			case "digest":
				release.Digest = ""
			}
			if _, err := svc.downloadTheme(context.Background(), release); err == nil {
				t.Fatal("missing identity accepted")
			}
			if requested || len(rt.startCalls) > 0 || len(svc.List()) > 0 {
				t.Fatal("incomplete identity admitted")
			}
		})
	}
}

func TestThemeMarketplaceUpdateRejectsInstalledKindMismatch(t *testing.T) {
	svc, _, rt := newTestService(t)
	installTestPlugin(t, svc, "runtime-control")
	before, _ := svc.registry.Get("runtime-control")
	if _, err := svc.installMarketplaceUpdate(context.Background(), marketplace.CatalogEntry{IndexEntry: marketplace.IndexEntry{ID: before.ID, Kind: "theme", Version: "2.0.0", PackageURL: "https://example.test/theme.tar.gz"}}); err == nil {
		t.Fatal("theme listing replaced an executable plugin")
	}
	after, _ := svc.registry.Get(before.ID)
	if !reflect.DeepEqual(before, after) || len(rt.startCalls) != 1 || len(rt.stopCalls) != 0 {
		t.Fatal("kind mismatch changed installed plugin")
	}
}

// @covers AC-PLUGINS-THEMES-003.1, AC-PLUGINS-THEMES-003.3
func TestThemeMarketplaceUpdateReviewIdentity(t *testing.T) {
	for _, scenario := range []string{"valid", "id", "version", "digest", "executable"} {
		t.Run(scenario, func(t *testing.T) {
			svc, _, rt := newTestService(t)
			rec, err := svc.Install(context.Background(), testThemeArchive(t))
			if err != nil {
				t.Fatal(err)
			}
			inventory := svc.ThemeInventory()
			data := themeUpdateArchive(t, rec.ID, "2.0.0")
			if scenario == "executable" {
				data = testPackage(t, "unexpected-runtime", "2.0.0", true).Bytes()
			}
			sum := sha256.Sum256(data)
			request := themeURLRequest{URL: "https://example.test/theme.tar.gz", ID: rec.ID, Version: "2.0.0", Digest: hex.EncodeToString(sum[:])}
			switch scenario {
			case "id":
				request.ID = "another-theme"
			case "version":
				request.Version = "3.0.0"
			case "digest":
				request.Digest = strings.Repeat("0", 64)
			}
			svc.httpClient = &http.Client{Transport: themeUpdateTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), Header: make(http.Header)}, nil
			})}
			body, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			response := doRequest(registerAdminPluginRoutes(t, svc), http.MethodPost, "/api/plugins/themes/prepare", string(body), map[string]string{"Content-Type": "application/json"})
			expected := http.StatusBadRequest
			if scenario == "valid" {
				expected = http.StatusOK
			}
			if response.Code != expected {
				t.Fatalf("status %d: %s", response.Code, response.Body.String())
			}
			after, _ := svc.registry.Get(rec.ID)
			if !reflect.DeepEqual(rec, after) || !reflect.DeepEqual(inventory, svc.ThemeInventory()) || len(rt.startCalls) > 0 || len(rt.stopCalls) > 0 {
				t.Fatal("review changed installed theme or invoked runtime")
			}
			if scenario != "valid" && len(svc.themeReviews.entries) != 0 {
				t.Fatal("invalid release left a committable review")
			}
		})
	}
}
