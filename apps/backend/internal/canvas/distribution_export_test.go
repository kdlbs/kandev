package canvas

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	plugininstances "github.com/kandev/kandev/internal/plugins/instances"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/webapp"
)

func TestPrepareExportBuildsOneReleaseBoundBundleAndSource(t *testing.T) {
	service, releases, _ := newDistributionExportTestService(t)
	review, err := service.PrepareExport(context.Background(), ExportRequest{
		UserID: "user-1", WorkspaceID: "workspace-1", CanvasID: "canvas-1", ExpectedReleaseID: "release-1",
		Metadata: ExportMetadata{PackageID: "canvas-board", Version: "1.2.3", DisplayName: "Board", Description: "A board", Author: "Kandev", License: "MIT", SourceMode: manifest.SourceModeStatic, MinKandevVersion: "1.0.0"},
	})
	if err != nil {
		t.Fatalf("PrepareExport() error = %v", err)
	}
	if review.PreparationID == "" || review.Digest == "" || review.BundleBytes == 0 || review.SourceBytes == 0 {
		t.Fatalf("review = %+v", review)
	}
	if review.Metadata.SourceMode != manifest.SourceModeStatic {
		t.Fatalf("metadata = %+v", review.Metadata)
	}
	if !hasExportFile(review.Files, "ui/logo.svg") || !hasExportFile(review.Files, "ui/body.woff2") {
		t.Fatalf("runtime assets missing from export inventory: %+v", review.Files)
	}
	if releases.lastID != "release-1" {
		t.Fatalf("release lookup = %q, want release-1", releases.lastID)
	}
	bundle, err := service.DownloadExport(context.Background(), "user-1", review.PreparationID, ExportBundle)
	if err != nil {
		t.Fatalf("DownloadExport(bundle) error = %v", err)
	}
	if _, err := webapp.ValidateDistributionPackage(bytes.NewReader(bundle.Data)); err != nil {
		t.Fatalf("downloaded bundle validation error = %v", err)
	}
	source, err := service.DownloadExport(context.Background(), "user-1", review.PreparationID, ExportSource)
	if err != nil {
		t.Fatalf("DownloadExport(source) error = %v", err)
	}
	if _, err := zip.NewReader(bytes.NewReader(source.Data), int64(len(source.Data))); err != nil {
		t.Fatalf("downloaded source validation error = %v", err)
	}
	if !sameReleaseID(review, "release-1") {
		t.Fatal("review is not bound to the expected release")
	}
}

func TestPrepareExportAllowsTaskCanvasWithPortableWorkspacePlacement(t *testing.T) {
	service, releases, canvases := newDistributionExportTestService(t)
	var sourceManifest manifest.Manifest
	if err := json.Unmarshal(releases.release.ManifestJSON, &sourceManifest); err != nil {
		t.Fatalf("unmarshal source manifest: %v", err)
	}
	sourceManifest.UI.WebApps[0].Placements = []string{manifest.WebAppPlacementTask}
	if err := sourceManifest.Validate(); err != nil {
		t.Fatalf("task source manifest should remain locally valid: %v", err)
	}
	releases.release.ManifestJSON, _ = json.Marshal(sourceManifest)
	canvases.canvas.ScopeKind = ScopeTask

	review, err := service.PrepareExport(context.Background(), ExportRequest{
		UserID: "user-1", WorkspaceID: "workspace-1", CanvasID: "canvas-1", ExpectedReleaseID: "release-1",
		Metadata: ExportMetadata{PackageID: "canvas-board", Version: "1.2.3", DisplayName: "Board", Description: "A board", Author: "Kandev", License: "MIT", SourceMode: manifest.SourceModeStatic, MinKandevVersion: "1.0.0"},
	})
	if err != nil {
		t.Fatalf("PrepareExport() error = %v", err)
	}
	if canvases.canvas.ScopeKind != ScopeTask {
		t.Fatalf("task canvas scope changed to %q during export", canvases.canvas.ScopeKind)
	}
	bundle, err := service.DownloadExport(context.Background(), "user-1", review.PreparationID, ExportBundle)
	if err != nil {
		t.Fatalf("DownloadExport() error = %v", err)
	}
	pkg, err := webapp.ValidateDistributionPackage(bytes.NewReader(bundle.Data))
	if err != nil {
		t.Fatalf("downloaded task bundle validation error = %v", err)
	}
	placements := pkg.Manifest.UI.WebApps[0].Placements
	if !containsString(placements, manifest.WebAppPlacementWorkspace) {
		t.Fatalf("exported placements = %v, want workspace-canvas", placements)
	}
}

func TestPrepareExportRejectsStaleReleaseWithoutCreatingPreparation(t *testing.T) {
	service, _, canvases := newDistributionExportTestService(t)
	canvases.canvas.ActiveReleaseID = "release-new"
	_, err := service.PrepareExport(context.Background(), ExportRequest{
		UserID: "user-1", WorkspaceID: "workspace-1", CanvasID: "canvas-1", ExpectedReleaseID: "release-1",
		Metadata: ExportMetadata{PackageID: "canvas-board", Version: "1.2.3", DisplayName: "Board", Description: "A board", Author: "Kandev", License: "MIT", SourceMode: manifest.SourceModeStatic, MinKandevVersion: "1.0.0"},
	})
	if !errors.Is(err, ErrExportStale) {
		t.Fatalf("PrepareExport() error = %v, want ErrExportStale", err)
	}
	if service.preparationsCount() != 0 {
		t.Fatal("stale export created a preparation")
	}
}

func TestPrepareInstallInspectsExactBundleAndReturnsPermissions(t *testing.T) {
	service, _, _ := newDistributionExportTestService(t)
	export, err := service.PrepareExport(context.Background(), ExportRequest{
		UserID: "user-1", WorkspaceID: "workspace-1", CanvasID: "canvas-1", ExpectedReleaseID: "release-1",
		Metadata: ExportMetadata{PackageID: "canvas-board", Version: "1.2.3", DisplayName: "Board", Description: "A board", Author: "Kandev", License: "MIT", SourceMode: manifest.SourceModeStatic, MinKandevVersion: "1.0.0"},
	})
	if err != nil {
		t.Fatalf("PrepareExport() error = %v", err)
	}
	bundle, err := service.DownloadExport(context.Background(), "user-1", export.PreparationID, ExportBundle)
	if err != nil {
		t.Fatalf("DownloadExport() error = %v", err)
	}
	review, err := service.PrepareInstall(context.Background(), InstallRequest{
		UserID: "user-1", WorkspaceID: "workspace-1", OriginKind: "upload", Bundle: bundle.Data,
	})
	if err != nil {
		t.Fatalf("PrepareInstall() error = %v", err)
	}
	if review.Metadata.PackageID != "canvas-board" || review.Metadata.License != "MIT" || review.PermissionSummary.Reads == nil {
		t.Fatalf("install review = %+v", review)
	}
}

func TestPrepareInstallRejectsExpectedDigestMismatch(t *testing.T) {
	service, _, _ := newDistributionExportTestService(t)
	export, err := service.PrepareExport(context.Background(), ExportRequest{
		UserID: "user-1", WorkspaceID: "workspace-1", CanvasID: "canvas-1", ExpectedReleaseID: "release-1",
		Metadata: ExportMetadata{PackageID: "canvas-board", Version: "1.2.3", DisplayName: "Board", Description: "A board", Author: "Kandev", License: "MIT", SourceMode: manifest.SourceModeStatic, MinKandevVersion: "1.0.0"},
	})
	if err != nil {
		t.Fatalf("PrepareExport() error = %v", err)
	}
	bundle, err := service.DownloadExport(context.Background(), "user-1", export.PreparationID, ExportBundle)
	if err != nil {
		t.Fatalf("DownloadExport() error = %v", err)
	}
	_, err = service.PrepareInstall(context.Background(), InstallRequest{UserID: "user-1", WorkspaceID: "workspace-1", OriginKind: "upload", ExpectedDigest: "wrong", Bundle: bundle.Data})
	if !errors.Is(err, ErrPackageDigestMismatch) {
		t.Fatalf("PrepareInstall() error = %v, want ErrPackageDigestMismatch", err)
	}
}

func TestConfirmInstallChecksDurableArtifactQuotaBeforeWriting(t *testing.T) {
	exportService, _, _ := newDistributionExportTestService(t)
	export, err := exportService.PrepareExport(context.Background(), ExportRequest{
		UserID: "user-1", WorkspaceID: "workspace-1", CanvasID: "canvas-1", ExpectedReleaseID: "release-1",
		Metadata: ExportMetadata{PackageID: "canvas-board", Version: "1.2.3", DisplayName: "Board", Description: "A board", Author: "Kandev", License: "MIT", SourceMode: manifest.SourceModeStatic, MinKandevVersion: "1.0.0"},
	})
	if err != nil {
		t.Fatalf("PrepareExport() error = %v", err)
	}
	bundle, err := exportService.DownloadExport(context.Background(), "user-1", export.PreparationID, ExportBundle)
	if err != nil {
		t.Fatalf("DownloadExport() error = %v", err)
	}
	installer := &fakeDistributionInstaller{}
	artifactWriter := &fakeDistributionArtifactWriter{}
	quota := &fakeDistributionQuota{reserveErr: plugininstances.ErrWorkspaceStorageLimit}
	service := NewDistributionService(installer, &fakeDistributionReleaseReader{}, artifactWriter, func(context.Context, string) error { return nil }, exportService.preparations)
	service.SetArtifactQuota(quota)
	review, err := service.PrepareInstall(context.Background(), InstallRequest{UserID: "user-1", WorkspaceID: "workspace-1", OriginKind: "upload", Bundle: bundle.Data})
	if err != nil {
		t.Fatalf("PrepareInstall() error = %v", err)
	}
	_, err = service.ConfirmInstall(context.Background(), "user-1", review.PreparationID, InstallConfirmation{ExpectedDigest: review.Digest, Approved: true})
	if !errors.Is(err, plugininstances.ErrWorkspaceStorageLimit) {
		t.Fatalf("ConfirmInstall() error = %v, want workspace storage limit", err)
	}
	if artifactWriter.next != 0 || installer.created != 0 || quota.released != 0 {
		t.Fatalf("quota failure left writes behind: artifacts=%d installs=%d releases=%d", artifactWriter.next, installer.created, quota.released)
	}
}

func TestConfirmInstallReleasesDurableArtifactReservationAfterInstall(t *testing.T) {
	exportService, _, _ := newDistributionExportTestService(t)
	export, err := exportService.PrepareExport(context.Background(), ExportRequest{
		UserID: "user-1", WorkspaceID: "workspace-1", CanvasID: "canvas-1", ExpectedReleaseID: "release-1",
		Metadata: ExportMetadata{PackageID: "canvas-board", Version: "1.2.3", DisplayName: "Board", Description: "A board", Author: "Kandev", License: "MIT", SourceMode: manifest.SourceModeStatic, MinKandevVersion: "1.0.0"},
	})
	if err != nil {
		t.Fatalf("PrepareExport() error = %v", err)
	}
	bundle, err := exportService.DownloadExport(context.Background(), "user-1", export.PreparationID, ExportBundle)
	if err != nil {
		t.Fatalf("DownloadExport() error = %v", err)
	}
	quota := &fakeDistributionQuota{}
	installer := &fakeDistributionInstaller{}
	writer := &fakeDistributionArtifactWriter{}
	service := NewDistributionService(installer, &fakeDistributionReleaseReader{}, writer, func(context.Context, string) error { return nil }, exportService.preparations)
	service.SetArtifactQuota(quota)
	review, err := service.PrepareInstall(context.Background(), InstallRequest{UserID: "user-1", WorkspaceID: "workspace-1", OriginKind: "upload", Bundle: bundle.Data})
	if err != nil {
		t.Fatalf("PrepareInstall() error = %v", err)
	}
	if _, err := service.ConfirmInstall(context.Background(), "user-1", review.PreparationID, InstallConfirmation{ExpectedDigest: review.Digest, Approved: true}); err != nil {
		t.Fatalf("ConfirmInstall() error = %v", err)
	}
	if quota.released != 1 || writer.next != 1 || installer.created != 1 {
		t.Fatalf("reservation cleanup = released %d, artifacts %d, installs %d", quota.released, writer.next, installer.created)
	}
}

func TestPrepareInstallRejectsPackageRequiringNewerKandev(t *testing.T) {
	service, _, _ := newDistributionExportTestService(t)
	service.SetKandevVersion("0.9.0")
	export, err := service.PrepareExport(context.Background(), ExportRequest{
		UserID: "user-1", WorkspaceID: "workspace-1", CanvasID: "canvas-1", ExpectedReleaseID: "release-1",
		Metadata: ExportMetadata{PackageID: "canvas-board", Version: "1.2.3", DisplayName: "Board", Description: "A board", Author: "Kandev", License: "MIT", SourceMode: manifest.SourceModeStatic, MinKandevVersion: "1.0.0"},
	})
	if err != nil {
		t.Fatalf("PrepareExport() error = %v", err)
	}
	bundle, err := service.DownloadExport(context.Background(), "user-1", export.PreparationID, ExportBundle)
	if err != nil {
		t.Fatalf("DownloadExport() error = %v", err)
	}
	_, err = service.PrepareInstall(context.Background(), InstallRequest{UserID: "user-1", WorkspaceID: "workspace-1", OriginKind: "upload", Bundle: bundle.Data})
	if !errors.Is(err, ErrInstallIncompatible) {
		t.Fatalf("PrepareInstall() error = %v, want ErrInstallIncompatible", err)
	}
}

func TestConfirmInstallIsIdempotentForOnePreparation(t *testing.T) {
	exportService, _, _ := newDistributionExportTestService(t)
	export, err := exportService.PrepareExport(context.Background(), ExportRequest{
		UserID: "user-1", WorkspaceID: "workspace-1", CanvasID: "canvas-1", ExpectedReleaseID: "release-1",
		Metadata: ExportMetadata{PackageID: "canvas-board", Version: "1.2.3", DisplayName: "Board", Description: "A board", Author: "Kandev", License: "MIT", SourceMode: manifest.SourceModeStatic, MinKandevVersion: "1.0.0"},
	})
	if err != nil {
		t.Fatalf("PrepareExport() error = %v", err)
	}
	bundle, err := exportService.DownloadExport(context.Background(), "user-1", export.PreparationID, ExportBundle)
	if err != nil {
		t.Fatalf("DownloadExport() error = %v", err)
	}
	installer := &fakeDistributionInstaller{}
	artifactWriter := &fakeDistributionArtifactWriter{}
	service := NewDistributionService(installer, &fakeDistributionReleaseReader{}, artifactWriter, func(context.Context, string) error { return nil }, exportService.preparations)
	review, err := service.PrepareInstall(context.Background(), InstallRequest{UserID: "user-1", WorkspaceID: "workspace-1", OriginKind: "upload", Bundle: bundle.Data})
	if err != nil {
		t.Fatalf("PrepareInstall() error = %v", err)
	}
	first, err := service.ConfirmInstall(context.Background(), "user-1", review.PreparationID, InstallConfirmation{ExpectedDigest: review.Digest, Approved: true})
	if err != nil {
		t.Fatalf("ConfirmInstall(first) error = %v", err)
	}
	second, err := service.ConfirmInstall(context.Background(), "user-1", review.PreparationID, InstallConfirmation{ExpectedDigest: review.Digest, Approved: true})
	if err != nil {
		t.Fatalf("ConfirmInstall(retry) error = %v", err)
	}
	if first.Canvas == nil || second.Canvas == nil || first.Canvas.ID != second.Canvas.ID || installer.created != 1 {
		t.Fatalf("first=%+v second=%+v created=%d", first, second, installer.created)
	}
}

type fakeDistributionCanvasReader struct{ canvas *Canvas }

func (f *fakeDistributionCanvasReader) Get(context.Context, string) (*Canvas, error) {
	return f.canvas, nil
}

type fakeDistributionReleaseReader struct {
	release plugininstances.Release
	lastID  string
}

func (f *fakeDistributionReleaseReader) GetRelease(_ context.Context, id string) (plugininstances.Release, error) {
	f.lastID = id
	return f.release, nil
}

type fakeDistributionArtifactReader struct{ files map[string][]byte }

func (f *fakeDistributionArtifactReader) ReadFiles(webapp.Artifact) (map[string][]byte, error) {
	return f.files, nil
}

type fakeDistributionArtifactWriter struct{ next int }

func (f *fakeDistributionArtifactWriter) ReadFiles(webapp.Artifact) (map[string][]byte, error) {
	return nil, nil
}

func (f *fakeDistributionArtifactWriter) Put(pkg *webapp.Package) (webapp.Artifact, error) {
	f.next++
	return webapp.Artifact{Digest: pkg.Digest, RelativePath: "releases/" + pkg.Digest, Bytes: pkg.ExpandedBytes, Available: true}, nil
}

type fakeDistributionQuota struct {
	reserveErr error
	released   int
}

func (f *fakeDistributionQuota) ReserveBytes(context.Context, string, int64, int64, int64) (plugininstances.Reservation, error) {
	if f.reserveErr != nil {
		return plugininstances.Reservation{}, f.reserveErr
	}
	return plugininstances.Reservation{ID: "reservation-1"}, nil
}

func (f *fakeDistributionQuota) ReleaseBytes(context.Context, string) error {
	f.released++
	return nil
}

type fakeDistributionInstaller struct {
	created int
	canvas  *Canvas
}

func (f *fakeDistributionInstaller) InstallCanvasPackage(_ context.Context, request InstallCanvasPackageRequest) (*InstallResult, error) {
	f.created++
	f.canvas = &Canvas{ID: "installed-canvas", PluginInstanceID: "installed-instance", WorkspaceID: request.WorkspaceID, ScopeKind: ScopeWorkspace, Status: StatusActive, Title: request.Title}
	receipt := request.Receipt
	receipt.CanvasID = f.canvas.ID
	return &InstallResult{Canvas: f.canvas, Receipt: receipt}, nil
}

func (f *fakeDistributionInstaller) Get(context.Context, string) (*Canvas, error) {
	if f.canvas == nil {
		return nil, ErrCanvasNotFound
	}
	return f.canvas, nil
}

func (f *fakeDistributionInstaller) CreateCanvas(_ context.Context, request CreateCanvasRequest) (*Canvas, error) {
	f.created++
	f.canvas = &Canvas{ID: "installed-canvas", PluginInstanceID: "installed-instance", WorkspaceID: request.WorkspaceID, ScopeKind: ScopeWorkspace, Status: StatusActive, Title: request.Title}
	return f.canvas, nil
}

func (f *fakeDistributionInstaller) PublishPackage(_ context.Context, request PublishRequest) (*PublishResult, error) {
	return &PublishResult{Canvas: f.canvas, Release: plugininstances.Release{ID: "installed-release", InstanceID: f.canvas.PluginInstanceID}, Activated: false}, nil
}

func (f *fakeDistributionInstaller) ApproveRelease(context.Context, string, string, string) (*Canvas, error) {
	f.canvas.ActiveReleaseID = "installed-release"
	f.canvas.ActiveReleaseStatus = ValidationValid
	return f.canvas, nil
}

func newDistributionExportTestService(t *testing.T) (*DistributionService, *fakeDistributionReleaseReader, *fakeDistributionCanvasReader) {
	t.Helper()
	manifestValue := manifest.Manifest{
		ID: "legacy-board", APIVersion: manifest.CurrentAPIVersion, Version: "0.1.0", DisplayName: "Board", Description: "A board", Author: "Author",
		UI:           manifest.UISection{WebApps: []manifest.WebApp{{Key: "main", Title: "Board", Entry: "ui/index.html", Placements: []string{manifest.WebAppPlacementWorkspace}}}},
		Capabilities: manifest.Capabilities{APIRead: []string{"tasks"}},
	}
	manifestJSON, err := json.Marshal(manifestValue)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	releases := &fakeDistributionReleaseReader{release: plugininstances.Release{
		ID: "release-1", InstanceID: "instance-1", PackageDigest: "old-digest", ArtifactPath: "releases/old-digest", ArtifactBytes: 10,
		ManifestJSON: manifestJSON, ValidationStatus: plugininstances.ValidationValid, CreatedAt: time.Now().UTC(),
	}}
	canvases := &fakeDistributionCanvasReader{canvas: &Canvas{ID: "canvas-1", WorkspaceID: "workspace-1", ScopeKind: ScopeWorkspace, Status: StatusActive, ActiveReleaseID: "release-1", ActiveReleaseStatus: ValidationValid}}
	files := map[string][]byte{
		"manifest.yaml":   []byte("legacy"),
		"ui/index.html":   []byte("<html></html>"),
		"ui/logo.svg":     []byte("<svg></svg>"),
		"ui/body.woff2":   []byte("font-bytes"),
		"ui/theme.css":    []byte("body { color: red; }"),
		"ui/runtime.json": []byte(`{"ready":true}`),
	}
	preparations, err := NewPreparationStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewPreparationStore: %v", err)
	}
	service := NewDistributionService(canvases, releases, &fakeDistributionArtifactReader{files: files}, func(context.Context, string) error { return nil }, preparations)
	return service, releases, canvases
}

func sameReleaseID(review ExportReview, id string) bool { return review.ReleaseID == id }

func hasExportFile(files []ExportFile, path string) bool {
	for _, file := range files {
		if file.Path == path {
			return true
		}
	}
	return false
}
