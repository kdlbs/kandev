package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/plugins/marketplace"
	"github.com/kandev/kandev/internal/plugins/provenance"
	"github.com/kandev/kandev/internal/plugins/store"
)

func TestPublisherExistingVersionVerification(t *testing.T) {
	svc, dir, fsStore, rt := newTestServiceWithDir(t)
	packageBytes := testPackage(t, "existing-plugin", "1.0.0", true).Bytes()
	rec, err := svc.InstallUploaded(context.Background(), strings.NewReader(string(packageBytes)))
	if err != nil {
		t.Fatalf("InstallUploaded: %v", err)
	}
	startCalls := rt.startCallCount(rec.ID)
	indexServer, packageURL := publisherReferenceServers(t, packageBytes, rec.ID, rec.Version)
	attachCanonicalMarketplace(t, svc, fsStore, indexServer, packageURL, packageBytes)

	verified, err := svc.VerifyInstalledPublisher(context.Background(), rec.ID, rec.InstallationID, rec.Version)
	if err != nil {
		t.Fatalf("VerifyInstalledPublisher: %v", err)
	}
	if verified.PublisherIdentity == nil || verified.PublisherIdentity.Status != provenance.StatusVerified || verified.PublisherIdentity.Login != "kdlbs" {
		t.Fatalf("verified identity = %+v, want verified kdlbs", verified.PublisherIdentity)
	}
	if verified.PublisherProvenance == nil || verified.PublisherProvenance.Origin != provenance.OriginUpload || verified.PublisherProvenance.VerificationMethod != provenance.VerificationInstalledFiles {
		t.Fatalf("verification changed origin/method incorrectly: %+v", verified.PublisherProvenance)
	}
	if got := rt.startCallCount(rec.ID); got != startCalls+1 || !rt.stopped(rec.ID) || !rt.Running(rec.ID) {
		t.Fatalf("verification did not pause and restore lifecycle: start calls %d (want %d), stopped=%v, running=%v", got, startCalls+1, rt.stopped(rec.ID), rt.Running(rec.ID))
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("plugin directory disappeared: %v", err)
	}

	// A fresh service reloads the same record and retains the host-owned
	// projection without consulting the catalog.
	reg := NewRegistry()
	if err := reg.Load(fsStore); err != nil {
		t.Fatalf("reload registry: %v", err)
	}
	svc2 := NewService(fsStore, reg, nil, testLogger(t))
	if err := svc2.SetPluginsDir(dir); err != nil {
		t.Fatalf("reload plugins dir: %v", err)
	}
	loaded, err := svc2.Get(rec.ID)
	if err != nil {
		t.Fatalf("reload get: %v", err)
	}
	if loaded.PublisherIdentity == nil || loaded.PublisherIdentity.Status != provenance.StatusVerified {
		t.Fatalf("reloaded identity = %+v, want verified", loaded.PublisherIdentity)
	}
}

func TestPublisherExistingVersionVerificationRejectsChangedFiles(t *testing.T) {
	svc, _, _, _ := newTestServiceWithDir(t)
	rec, err := svc.InstallUploaded(context.Background(), testPackage(t, "existing-mismatch", "1.0.0", false))
	if err != nil {
		t.Fatalf("InstallUploaded: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rec.InstallPath, "server", "plugin"), []byte("changed"), 0o755); err != nil {
		t.Fatalf("mutate installed file: %v", err)
	}
	indexServer, packageURL := publisherReferenceServers(t, testPackage(t, rec.ID, rec.Version, false).Bytes(), rec.ID, rec.Version)
	attachCanonicalMarketplace(t, svc, nil, indexServer, packageURL, testPackage(t, rec.ID, rec.Version, false).Bytes())

	_, err = svc.VerifyInstalledPublisher(context.Background(), rec.ID, rec.InstallationID, rec.Version)
	if !errors.Is(err, ErrInstalledPackageMismatch) {
		t.Fatalf("changed files error = %v, want ErrInstalledPackageMismatch", err)
	}
	got, getErr := svc.Get(rec.ID)
	if getErr != nil {
		t.Fatalf("Get after mismatch: %v", getErr)
	}
	if got.PublisherIdentity == nil || got.PublisherIdentity.Status != provenance.StatusUnverified {
		t.Fatalf("mismatch changed publisher identity: %+v", got.PublisherIdentity)
	}
}

func TestCompareInstalledPackageRejectsSameSizeMutationAfterRead(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "plugin.bin")
	want := []byte("original-bytes")
	mutated := []byte("mutated-bytes!")
	if len(want) != len(mutated) {
		t.Fatalf("test fixture lengths differ: %d != %d", len(want), len(mutated))
	}
	if err := os.WriteFile(path, want, 0o755); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	err := compareInstalledPackageWithAfterReadHook(root, map[string][]byte{"plugin.bin": want}, map[string]os.FileMode{"plugin.bin": 0o755}, func(path string) {
		if err := os.WriteFile(path, mutated, 0o755); err != nil {
			t.Fatalf("same-size mutation: %v", err)
		}
		stamp := time.Now().Add(time.Hour)
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatalf("stamp same-size mutation: %v", err)
		}
	})
	if !errors.Is(err, ErrInstalledPackageChanged) {
		t.Fatalf("same-size concurrent mutation error = %v, want ErrInstalledPackageChanged", err)
	}
}

func TestCompareInstalledPackageRejectsInstallerModeMismatch(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "plugin.bin")
	want := []byte("plugin")
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	err := compareInstalledPackage(root, map[string][]byte{"plugin.bin": want}, map[string]os.FileMode{"plugin.bin": 0o755})
	if !errors.Is(err, ErrInstalledPackageMismatch) {
		t.Fatalf("mode mismatch error = %v, want ErrInstalledPackageMismatch", err)
	}
}

func TestPublisherExistingVersionVerificationRequiresExactListedVersion(t *testing.T) {
	svc, _, _, _ := newTestServiceWithDir(t)
	rec := installTestPlugin(t, svc, "existing-unavailable")
	index := `{"schema_version":1,"source":{"name":"Kandev Official","url":""},"plugins":[]}`
	indexServer := newStaticHTTPClient(t, marketplace.OfficialSourceURL, []byte(index), "")
	attachCanonicalMarketplaceWithClient(t, svc, indexServer)

	_, err := svc.VerifyInstalledPublisher(context.Background(), rec.ID, rec.InstallationID, rec.Version)
	if !errors.Is(err, ErrPublisherEvidenceUnavailable) {
		t.Fatalf("missing version error = %v, want ErrPublisherEvidenceUnavailable", err)
	}
}

func publisherReferenceServers(t *testing.T, packageBytes []byte, id, version string) (*staticHTTPClient, string) {
	t.Helper()
	digest := sha256.Sum256(packageBytes)
	digestText := hex.EncodeToString(digest[:])
	packageURL := "https://cdn.example/" + id + ".tar.gz"
	index := `{"schema_version":1,"source":{"name":"Kandev Official","url":"` + marketplace.OfficialSourceURL + `"},"plugins":[` +
		`{"id":"` + id + `","kind":"plugin","name":"Existing","version":"` + version + `","package_sha256":"` + digestText + `","package_url":"` + packageURL + `","publisher":{"schema_version":1,"repository_id":"123","owner_id":"456","login":"kdlbs","repository":"kdlbs/` + id + `","official":true}}]}`
	return newStaticHTTPClient(t, marketplace.OfficialSourceURL, []byte(index), packageURL, packageBytes), packageURL
}

type staticHTTPClient struct {
	responses map[string][]byte
}

func (c *staticHTTPClient) RoundTrip(req *http.Request) (*http.Response, error) {
	body, ok := c.responses[req.URL.String()]
	if !ok {
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header), Request: req}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header), Request: req}, nil
}

func newStaticHTTPClient(t *testing.T, indexURL string, index []byte, packageURL string, packageBytes ...[]byte) *staticHTTPClient {
	t.Helper()
	responses := map[string][]byte{indexURL: index}
	if packageURL != "" && len(packageBytes) > 0 {
		responses[packageURL] = packageBytes[0]
	}
	return &staticHTTPClient{responses: responses}
}

func attachCanonicalMarketplace(t *testing.T, svc *Service, fsStore *store.FSStore, client *staticHTTPClient, packageURL string, packageBytes []byte) {
	t.Helper()
	_ = fsStore
	attachCanonicalMarketplaceWithClient(t, svc, client)
	if packageURL != "" && len(packageBytes) > 0 {
		svc.httpClient = &http.Client{Transport: client}
	}
}

func attachCanonicalMarketplaceWithClient(t *testing.T, svc *Service, client *staticHTTPClient) {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	sourceStore, err := marketplace.NewSourceStore(db.NewPool(conn, conn))
	if err != nil {
		t.Fatalf("source store: %v", err)
	}
	if err := sourceStore.EnsureBuiltin(marketplace.OfficialSourceName, marketplace.OfficialSourceURL); err != nil {
		t.Fatalf("ensure builtin: %v", err)
	}
	m := marketplace.NewService(sourceStore, testLogger(t))
	m.SetHTTPClient(&http.Client{Transport: client})
	svc.SetMarketplace(m)
}
