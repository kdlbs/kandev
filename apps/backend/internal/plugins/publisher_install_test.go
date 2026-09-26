package plugins

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/plugins/marketplace"
	"github.com/kandev/kandev/internal/plugins/provenance"
)

func TestPublisherCatalogInstall(t *testing.T) {
	svc, _, _, rt := newTestServiceWithDir(t)
	packageBytes := testPackage(t, "catalog-plugin", "1.2.3", false).Bytes()
	digest := sha256.Sum256(packageBytes)
	digestText := hex.EncodeToString(digest[:])
	packageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(packageBytes)
	}))
	t.Cleanup(packageServer.Close)
	index := `{"schema_version":1,"source":{"name":"custom","url":""},"plugins":[` +
		`{"id":"catalog-plugin","kind":"plugin","name":"Catalog Plugin","version":"1.2.3","package_sha256":"` + digestText + `","package_url":"` + packageServer.URL + `/catalog.tar.gz"}]}`
	indexServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(index))
	}))
	t.Cleanup(indexServer.Close)

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
	source, err := sourceStore.Add("Custom", indexServer.URL)
	if err != nil {
		t.Fatalf("add source: %v", err)
	}
	svc.SetMarketplace(marketplace.NewService(sourceStore, testLogger(t)))

	rec, err := svc.InstallFromCatalog(context.Background(), CatalogInstallSelector{
		SourceID:        source.ID,
		PackageID:       "catalog-plugin",
		ExpectedVersion: "1.2.3",
		ExpectedSHA256:  digestText,
	})
	if err != nil {
		t.Fatalf("InstallFromCatalog: %v", err)
	}
	if rec.Version != "1.2.3" || rec.ID != "catalog-plugin" || !rt.Running(rec.ID) {
		t.Fatalf("installed record = %+v, running=%v", rec, rt.Running(rec.ID))
	}
	if rec.PublisherProvenance == nil || rec.PublisherProvenance.Origin != provenance.OriginCatalog || rec.PublisherProvenance.SourceID != source.ID {
		t.Fatalf("catalog origin was not persisted: %+v", rec.PublisherProvenance)
	}
	if rec.PublisherIdentity == nil || rec.PublisherIdentity.Status != provenance.StatusUnverified {
		t.Fatalf("custom catalog publisher identity = %+v, want unverified", rec.PublisherIdentity)
	}
}

func TestInstallFromURLDoesNotPersistCredentialOrQueryData(t *testing.T) {
	svc, _, fsStore, _ := newTestServiceWithDir(t)
	packageBytes := testPackage(t, "direct-url-plugin", "1.0.0", false).Bytes()
	packageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(packageBytes)
	}))
	t.Cleanup(packageServer.Close)

	transportURL, err := url.Parse(packageServer.URL + "/direct.tar.gz")
	if err != nil {
		t.Fatalf("parse package URL: %v", err)
	}
	transportURL.User = url.UserPassword("download-user", "download-secret")
	transportURL.RawQuery = "X-Amz-Signature=signed-token&X-Amz-Credential=credential"
	transportURL.Fragment = "download-fragment"

	rec, err := svc.InstallFromURL(context.Background(), transportURL.String())
	if err != nil {
		t.Fatalf("InstallFromURL: %v", err)
	}
	wantPublicURL := packageServer.URL + "/direct.tar.gz"
	if got := rec.PublisherProvenance.SourceURL; got != wantPublicURL {
		t.Fatalf("public source URL = %q, want %q", got, wantPublicURL)
	}
	if strings.Contains(rec.PublisherProvenance.SourceURL, "download-secret") || strings.Contains(rec.PublisherProvenance.SourceURL, "Signature") {
		t.Fatalf("credential-bearing URL leaked into returned provenance: %q", rec.PublisherProvenance.SourceURL)
	}
	onDisk, err := fsStore.Get(rec.ID)
	if err != nil {
		t.Fatalf("read persisted record: %v", err)
	}
	if got := onDisk.PublisherProvenance.SourceURL; got != wantPublicURL {
		t.Fatalf("persisted source URL = %q, want %q", got, wantPublicURL)
	}
}

func TestPublisherCatalogInstallRejectsDigestMismatchBeforeLifecycleMutation(t *testing.T) {
	svc, _, _, rt := newTestServiceWithDir(t)
	packageBytes := testPackage(t, "catalog-mismatch", "1.0.0", false).Bytes()
	packageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(packageBytes)
	}))
	t.Cleanup(packageServer.Close)
	index := `{"schema_version":1,"source":{"name":"custom","url":""},"plugins":[` +
		`{"id":"catalog-mismatch","version":"1.0.0","package_sha256":"` + strings.Repeat("a", 64) + `","package_url":"` + packageServer.URL + `/catalog.tar.gz"}]}`
	indexServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(index))
	}))
	t.Cleanup(indexServer.Close)

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
	source, err := sourceStore.Add("Custom", indexServer.URL)
	if err != nil {
		t.Fatalf("add source: %v", err)
	}
	svc.SetMarketplace(marketplace.NewService(sourceStore, testLogger(t)))

	if _, err := svc.InstallFromCatalog(context.Background(), CatalogInstallSelector{
		SourceID:        source.ID,
		PackageID:       "catalog-mismatch",
		ExpectedVersion: "1.0.0",
		ExpectedSHA256:  strings.Repeat("a", 64),
	}); err == nil {
		t.Fatal("InstallFromCatalog accepted a downloaded archive with a mismatched digest")
	}
	if len(svc.List()) != 0 || rt.Running("catalog-mismatch") {
		t.Fatalf("digest mismatch mutated lifecycle: records=%v running=%v", svc.List(), rt.Running("catalog-mismatch"))
	}
}

func TestInstallHandlerRejectsMixedURLCatalogAndPublisherFields(t *testing.T) {
	router, _ := newTestRouterWithIdentity(t, adminIdentity())
	for name, body := range map[string]string{
		"mixed selector":     `{"url":"https://example.test/plugin.tar.gz","catalog":{"source_id":"official","package_id":"example","expected_version":"1.0.0"}}`,
		"publisher evidence": `{"url":"https://example.test/plugin.tar.gz","publisher":{"status":"verified","login":"kdlbs"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			rec := doRequest(router, http.MethodPost, "/api/plugins/install", body, map[string]string{"Content-Type": "application/json"})
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
			}
			if json.Valid(rec.Body.Bytes()) && bytes.Contains(rec.Body.Bytes(), []byte(`"status":"verified"`)) {
				t.Fatal("handler echoed request publisher evidence")
			}
		})
	}
}

func adminIdentity() authn.Identity {
	return authn.Identity{UserID: "admin-1", Role: authn.RoleAdmin}
}
