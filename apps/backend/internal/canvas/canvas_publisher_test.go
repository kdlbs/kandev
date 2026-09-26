package canvas

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/db"
	plugininstances "github.com/kandev/kandev/internal/plugins/instances"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/provenance"
	"github.com/kandev/kandev/internal/plugins/webapp"
)

func TestCanvasCatalogInstallCarriesPublisherReceiptThroughReplay(t *testing.T) {
	bundle, pkg := publisherCanvasBundle(t)
	archive := archiveDigest(bundle)
	resolver := &publisherCanvasResolver{provenance: &provenance.InstallationProvenance{
		Origin: provenance.OriginCatalog, SourceID: "official", SourceURL: "https://official.example.test/index.json",
		PackageID: pkg.Manifest.ID, Version: pkg.Manifest.Version, PackageSHA256: archive,
		Publisher:  &provenance.Evidence{SchemaVersion: 1, RepositoryID: "100", OwnerID: "200", Login: "acme", Repository: "acme/canvas"},
		VerifiedAt: timePtr(time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)), VerificationMethod: provenance.VerificationArchiveDownload,
		MatchedSourceID: "official", MatchedSourceURL: "https://official.example.test/index.json",
	}}
	installer := &fakeDistributionInstaller{}
	preparations, err := NewPreparationStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewPreparationStore() error = %v", err)
	}
	service := NewDistributionService(installer, &fakeDistributionReleaseReader{}, &fakeDistributionArtifactWriter{}, func(context.Context, string) error { return nil }, preparations)
	service.SetCatalogResolver(resolver)
	service.SetInstallHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(bytes.NewReader(bundle)), ContentLength: int64(len(bundle)), Header: make(http.Header), Request: req}, nil
	})})

	review, err := service.PrepareInstallFromCatalog(context.Background(), InstallRequest{
		UserID: "user-1", WorkspaceID: "workspace-1", SourceID: "official", PackageID: pkg.Manifest.ID,
		ExpectedVersion: pkg.Manifest.Version, ExpectedArchiveDigest: archive,
	})
	if err != nil {
		t.Fatalf("PrepareInstallFromCatalog() error = %v", err)
	}
	if review.PublisherIdentity == nil || review.PublisherIdentity.Status != provenance.StatusVerified || review.PublisherIdentity.Login != "acme" {
		t.Fatalf("review publisher identity = %+v, want verified acme", review.PublisherIdentity)
	}
	preparation, err := preparations.Get(context.Background(), "user-1", review.PreparationID)
	if err != nil {
		t.Fatalf("get preparation: %v", err)
	}
	if preparation.PublisherProvenance == nil || preparation.PublisherProvenance.PackageSHA256 != archive {
		t.Fatalf("preparation provenance = %+v, want archive digest %s", preparation.PublisherProvenance, archive)
	}

	first, err := service.ConfirmInstall(context.Background(), "user-1", review.PreparationID, InstallConfirmation{ExpectedDigest: review.Digest, Approved: true})
	if err != nil {
		t.Fatalf("ConfirmInstall(first) error = %v", err)
	}
	second, err := service.ConfirmInstall(context.Background(), "user-1", review.PreparationID, InstallConfirmation{ExpectedDigest: review.Digest, Approved: true})
	if err != nil {
		t.Fatalf("ConfirmInstall(replay) error = %v", err)
	}
	if installer.created != 1 || first.Receipt.PublisherIdentity.Status != provenance.StatusVerified || second.Receipt.PublisherIdentity.Login != "acme" {
		t.Fatalf("created=%d first=%+v second=%+v, want one verified install and replay", installer.created, first.Receipt, second.Receipt)
	}
}

func TestCanvasUploadsRemainUnverifiedWhenSourceFieldsNameOfficialPublisher(t *testing.T) {
	bundle, _ := publisherCanvasBundle(t)
	preparations, err := NewPreparationStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewPreparationStore() error = %v", err)
	}
	service := NewDistributionService(&fakeDistributionInstaller{}, &fakeDistributionReleaseReader{}, &fakeDistributionArtifactWriter{}, func(context.Context, string) error { return nil }, preparations)
	review, err := service.PrepareInstall(context.Background(), InstallRequest{
		UserID: "user-1", WorkspaceID: "workspace-1", OriginKind: provenance.OriginUpload,
		SourceID: "official", RepositoryURL: "https://github.com/kdlbs/kandev", Bundle: bundle,
	})
	if err != nil {
		t.Fatalf("PrepareInstall() error = %v", err)
	}
	if review.PublisherIdentity == nil || review.PublisherIdentity.Status != provenance.StatusUnverified || review.PublisherIdentity.Login != "" {
		t.Fatalf("upload publisher identity = %+v, want unverified without login", review.PublisherIdentity)
	}
	preparation, err := preparations.Get(context.Background(), "user-1", review.PreparationID)
	if err != nil {
		t.Fatalf("get preparation: %v", err)
	}
	if preparation.PublisherProvenance == nil || preparation.PublisherProvenance.Publisher != nil {
		t.Fatalf("upload provenance = %+v, want no publisher evidence", preparation.PublisherProvenance)
	}
}

func TestCanvasPublisherReceiptMigrationAndReleaseProjection(t *testing.T) {
	pool := openCanvasPool(t, t.TempDir()+"/legacy-receipt.db")
	_, err := pool.Writer().Exec(`CREATE TABLE canvas_install_receipts (
		preparation_id TEXT PRIMARY KEY, user_id TEXT NOT NULL, canvas_id TEXT NOT NULL UNIQUE,
		workspace_id TEXT NOT NULL, package_id TEXT NOT NULL, package_version TEXT NOT NULL,
		package_digest TEXT NOT NULL, source_id TEXT NOT NULL DEFAULT '', repository_url TEXT NOT NULL DEFAULT '',
		origin_kind TEXT NOT NULL, created_at TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatalf("create legacy receipt table: %v", err)
	}
	if _, err := pool.Writer().Exec(`INSERT INTO canvas_install_receipts (preparation_id, user_id, canvas_id, workspace_id, package_id, package_version, package_digest, source_id, repository_url, origin_kind, created_at) VALUES ('legacy-prep', 'user-1', 'legacy-canvas', 'workspace-1', 'canvas-board', '1.0.0', 'legacy-digest', '', '', 'upload', '2026-09-18T12:00:00Z')`); err != nil {
		t.Fatalf("insert legacy receipt: %v", err)
	}
	repo, err := NewRepository(pool)
	if err != nil {
		t.Fatalf("upgrade repository: %v", err)
	}
	legacyProvenance := `{"origin":"upload","source_url":"https://user:secret@downloads.example/index.json?sig=private","package_id":"canvas-board","version":"1.0.0"}`
	if _, err := pool.Writer().Exec(`UPDATE canvas_install_receipts SET publisher_provenance_json = ? WHERE preparation_id = ?`, legacyProvenance, "legacy-prep"); err != nil {
		t.Fatalf("write legacy provenance: %v", err)
	}
	legacy, err := repo.GetInstallReceipt(context.Background(), "legacy-prep", "user-1")
	if err != nil {
		t.Fatalf("read legacy receipt: %v", err)
	}
	if legacy.PublisherIdentity == nil || legacy.PublisherIdentity.Status != provenance.StatusUnverified {
		t.Fatalf("legacy identity = %+v, want unverified", legacy.PublisherIdentity)
	}
	if legacy.PublisherProvenance == nil || legacy.PublisherProvenance.SourceURL != "https://downloads.example/index.json" {
		t.Fatalf("legacy receipt source URL = %+v, want sanitized URL", legacy.PublisherProvenance)
	}
	var releaseIndex string
	if err := pool.Reader().Get(&releaseIndex, `SELECT name FROM sqlite_master WHERE type = 'index' AND name = 'idx_canvas_install_receipts_release'`); err != nil {
		t.Fatalf("release index lookup: %v", err)
	}
	if releaseIndex != "idx_canvas_install_receipts_release" {
		t.Fatalf("release index = %q", releaseIndex)
	}
	columns, err := db.TableColumns(pool.Writer(), "canvas_install_receipts")
	if err != nil {
		t.Fatalf("inspect upgraded receipt columns: %v", err)
	}
	if !columns["release_id"] || !columns["publisher_provenance_json"] {
		t.Fatalf("upgraded receipt columns = %v", columns)
	}

	service, instanceStore, _ := newCanvasService(t)
	created := createCanvas(t, service, CreateCanvasRequest{WorkspaceID: "workspace-1", Title: "Imported canvas"})
	release := publisherRelease(t, "imported-release", created.PluginInstanceID, "normalized-imported")
	if err := instanceStore.CreateRelease(context.Background(), release); err != nil {
		t.Fatalf("create imported release: %v", err)
	}
	if err := instanceStore.SetActiveRelease(context.Background(), created.PluginInstanceID, release.ID); err != nil {
		t.Fatalf("activate imported release: %v", err)
	}
	publisher := publisherProvenance(strings.Repeat("c", 64), provenance.OriginCatalog)
	if err := service.repo.CreateInstallReceipt(context.Background(), InstallReceipt{
		PreparationID: "imported-prep", UserID: "user-1", CanvasID: created.ID, WorkspaceID: created.WorkspaceID,
		PackageID: "canvas-board", Version: "1.0.0", Digest: release.PackageDigest, ReleaseID: release.ID,
		OriginKind: provenance.OriginCatalog, PublisherProvenance: publisher,
	}); err != nil {
		t.Fatalf("create imported receipt: %v", err)
	}
	imported, err := service.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get imported canvas: %v", err)
	}
	if imported.ActiveRelease.PublisherIdentity.Status != provenance.StatusVerified {
		t.Fatalf("imported release identity = %+v, want verified", imported.ActiveRelease.PublisherIdentity)
	}

	local := publisherRelease(t, "local-edit", created.PluginInstanceID, "normalized-local")
	if err := instanceStore.CreateRelease(context.Background(), local); err != nil {
		t.Fatalf("create local release: %v", err)
	}
	if err := instanceStore.SetActiveRelease(context.Background(), created.PluginInstanceID, local.ID); err != nil {
		t.Fatalf("activate local release: %v", err)
	}
	edited, err := service.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get edited canvas: %v", err)
	}
	if edited.ActiveRelease.PublisherIdentity.Status != provenance.StatusUnverified {
		t.Fatalf("local release identity = %+v, want unverified", edited.ActiveRelease.PublisherIdentity)
	}
	if err := instanceStore.SetActiveRelease(context.Background(), created.PluginInstanceID, release.ID); err != nil {
		t.Fatalf("restore imported release: %v", err)
	}
	restored, err := service.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get restored canvas: %v", err)
	}
	if restored.ActiveRelease.PublisherIdentity.Status != provenance.StatusVerified {
		t.Fatalf("restored release identity = %+v, want verified", restored.ActiveRelease.PublisherIdentity)
	}
}

func TestCanvasPublisherInstallRollbackDoesNotLeaveReceipt(t *testing.T) {
	service, instanceStore, _ := newCanvasService(t)
	service.instances = &activationFailureStore{Store: instanceStore}
	_, err := service.InstallCanvasPackage(context.Background(), InstallCanvasPackageRequest{
		WorkspaceID: "workspace-1", Title: "Rollback publisher", Package: testCanvasPackage("rollback-publisher", nil),
		Artifact:        webapp.Artifact{Digest: "rollback-publisher", RelativePath: "releases/rollback-publisher", Bytes: 1},
		SourceActorKind: "canvas-install:catalog", SourceUserID: "user-1", Approved: true,
		Receipt: InstallReceipt{PreparationID: "rollback-prep", UserID: "user-1", PackageID: "canvas-board", Version: "rollback-publisher", Digest: "rollback-publisher", OriginKind: provenance.OriginCatalog, PublisherProvenance: publisherProvenance(strings.Repeat("a", 64), provenance.OriginCatalog)},
	})
	if err == nil {
		t.Fatal("InstallCanvasPackage() succeeded, want transaction failure")
	}
	if _, err := service.repo.GetInstallReceipt(context.Background(), "rollback-prep", "user-1"); !errors.Is(err, ErrInstallReceiptNotFound) {
		t.Fatalf("rollback receipt lookup = %v, want ErrInstallReceiptNotFound", err)
	}
}

type publisherCanvasResolver struct {
	provenance *provenance.InstallationProvenance
}

func (r *publisherCanvasResolver) ResolveCanvasPackage(context.Context, string, string, string, string) (string, string, error) {
	return "https://packages.example.test/canvas.tar.gz", "https://github.com/acme/canvas", nil
}

func (r *publisherCanvasResolver) ResolveCanvasPackageWithProvenance(context.Context, string, string, string, string) (string, string, *provenance.InstallationProvenance, error) {
	return "https://packages.example.test/canvas.tar.gz", "https://github.com/acme/canvas", r.provenance.Clone(), nil
}

func publisherCanvasBundle(t *testing.T) ([]byte, *webapp.Package) {
	t.Helper()
	pkg, err := webapp.BuildDistributionPackage(&manifest.Manifest{
		ID: "canvas-publisher", APIVersion: manifest.CurrentAPIVersion, Version: "1.0.0", DisplayName: "Publisher canvas", Description: "A publisher test canvas", Author: "Declared author", MinKandevVersion: "1.0.0",
		UI:           manifest.UISection{WebApps: []manifest.WebApp{{Key: "main", Title: "Publisher canvas", Entry: "ui/index.html", Placements: []string{manifest.WebAppPlacementWorkspace}}}},
		Distribution: &manifest.Distribution{SchemaVersion: manifest.DistributionSchemaVersion, Kind: manifest.DistributionKindCanvas, License: "Custom-License", SourceMode: manifest.SourceModeStatic},
	}, map[string][]byte{"README.md": []byte("Publisher test"), "LICENSE.txt": []byte("Publisher test license"), "ui/index.html": []byte("<html></html>")})
	if err != nil {
		t.Fatalf("BuildDistributionPackage() error = %v", err)
	}
	archives, err := webapp.BuildDistributionArchives(pkg)
	if err != nil {
		t.Fatalf("BuildDistributionArchives() error = %v", err)
	}
	return archives.Bundle, pkg
}

func publisherProvenance(archive, origin string) *provenance.InstallationProvenance {
	return &provenance.InstallationProvenance{
		Origin: origin, PackageID: "canvas-board", Version: "1.0.0", PackageSHA256: archive,
		Publisher:  &provenance.Evidence{SchemaVersion: 1, RepositoryID: "100", OwnerID: "200", Login: "acme", Repository: "acme/canvas"},
		VerifiedAt: timePtr(time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)), VerificationMethod: provenance.VerificationArchiveDownload,
		SourceID: "official", MatchedSourceID: "official", SourceURL: "https://official.example.test/index.json", MatchedSourceURL: "https://official.example.test/index.json",
	}
}

func publisherRelease(t *testing.T, id, instanceID, digest string) plugininstances.Release {
	t.Helper()
	manifestJSON, err := json.Marshal(testCanvasPackage("1.0.0", nil).Manifest)
	if err != nil {
		t.Fatalf("marshal publisher manifest: %v", err)
	}
	return plugininstances.Release{ID: id, PluginID: CanvasPluginID, InstanceID: instanceID, PackageDigest: digest, SourceKind: plugininstances.SourceLocalCanvas, ManifestJSON: manifestJSON, ArtifactPath: "releases/" + digest, ArtifactBytes: 1, ValidationStatus: ValidationValid, CreatedAt: time.Now().UTC()}
}

func timePtr(value time.Time) *time.Time {
	return &value
}
