package canvas

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	plugininstances "github.com/kandev/kandev/internal/plugins/instances"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/webapp"
)

var (
	ErrExportInvalid = errors.New("invalid canvas export")
	ErrExportStale   = errors.New("canvas export is stale")
)

type CanvasReader interface {
	Get(context.Context, string) (*Canvas, error)
}

type ReleaseReader interface {
	GetRelease(context.Context, string) (plugininstances.Release, error)
}

type ArtifactReader interface {
	ReadFiles(webapp.Artifact) (map[string][]byte, error)
}

type WorkspaceAuthorizer func(context.Context, string) error

// CatalogResolver resolves a server-selected catalog entry. It returns only
// trusted server metadata; the browser never supplies a package URL for this
// path.
type CatalogResolver interface {
	ResolveCanvasPackage(context.Context, string, string, string, string) (string, string, error)
}

type ExportMetadata struct {
	PackageID        string `json:"package_id"`
	Version          string `json:"version"`
	DisplayName      string `json:"display_name"`
	Description      string `json:"description"`
	Author           string `json:"author"`
	License          string `json:"license"`
	SourceMode       string `json:"source_mode"`
	MinKandevVersion string `json:"min_kandev_version"`
	RepoURL          string `json:"repo_url,omitempty"`
}

type ExportRequest struct {
	UserID            string
	WorkspaceID       string
	CanvasID          string
	ExpectedReleaseID string
	Metadata          ExportMetadata
}

type ExportReview struct {
	PreparationID  string         `json:"preparation_id"`
	CanvasID       string         `json:"canvas_id"`
	WorkspaceID    string         `json:"workspace_id"`
	ReleaseID      string         `json:"release_id"`
	Metadata       ExportMetadata `json:"metadata"`
	Digest         string         `json:"sha256"`
	Files          []ExportFile   `json:"files"`
	BundleBytes    int64          `json:"bundle_bytes"`
	SourceBytes    int64          `json:"source_bytes"`
	ExpiresAt      string         `json:"expires_at"`
	BundleDownload string         `json:"bundle_download"`
	SourceDownload string         `json:"source_download"`
}

type ExportDownload struct {
	Data        []byte
	Filename    string
	ContentType string
}

type DistributionService struct {
	canvases      CanvasReader
	releases      ReleaseReader
	artifacts     ArtifactReader
	authorize     WorkspaceAuthorizer
	preparations  *PreparationStore
	installMu     sync.Mutex
	receipts      map[string]InstallReceipt
	receiptStore  InstallReceiptStore
	httpClient    *http.Client
	catalog       CatalogResolver
	kandevVersion string
}

func NewDistributionService(canvases CanvasReader, releases ReleaseReader, artifacts ArtifactReader, authorize WorkspaceAuthorizer, preparations *PreparationStore) *DistributionService {
	return &DistributionService{canvases: canvases, releases: releases, artifacts: artifacts, authorize: authorize, preparations: preparations, receipts: make(map[string]InstallReceipt), httpClient: newDistributionHTTPClient()}
}

func (s *DistributionService) SetInstallReceiptStore(store InstallReceiptStore) {
	if s == nil {
		return
	}
	s.installMu.Lock()
	s.receiptStore = store
	s.installMu.Unlock()
}

// SetKandevVersion supplies the stamped server version used by the same
// compatibility gate as native plugin installation.
func (s *DistributionService) SetKandevVersion(version string) {
	if s == nil {
		return
	}
	s.installMu.Lock()
	s.kandevVersion = version
	s.installMu.Unlock()
}

// SetInstallHTTPClient is intended for deterministic local transport tests.
// Production composition leaves the destination-validating client installed.
func (s *DistributionService) SetInstallHTTPClient(client *http.Client) {
	if s == nil || client == nil {
		return
	}
	s.installMu.Lock()
	s.httpClient = client
	s.installMu.Unlock()
}

func (s *DistributionService) SetCatalogResolver(resolver CatalogResolver) {
	if s == nil {
		return
	}
	s.installMu.Lock()
	s.catalog = resolver
	s.installMu.Unlock()
}

func (s *DistributionService) PrepareExport(ctx context.Context, request ExportRequest) (ExportReview, error) {
	if err := s.validateReady(request.UserID, request.WorkspaceID, request.CanvasID); err != nil {
		return ExportReview{}, err
	}
	if err := s.authorizeWorkspace(ctx, request.WorkspaceID); err != nil {
		return ExportReview{}, err
	}
	item, release, files, err := s.loadExportSnapshot(ctx, request)
	if err != nil {
		return ExportReview{}, err
	}
	metadata, err := normalizeExportMetadata(request.Metadata, releaseManifest(release))
	if err != nil {
		return ExportReview{}, err
	}
	manifestValue, err := exportManifest(release.ManifestJSON, metadata)
	if err != nil {
		return ExportReview{}, err
	}
	if _, ok := files["README.md"]; !ok {
		files["README.md"] = []byte(fmt.Sprintf("# %s\n\n%s\n\nBuild and publish the retained canvas files.\n", metadata.DisplayName, metadata.Description))
	}
	pkg, err := webapp.BuildDistributionPackage(manifestValue, files)
	if err != nil {
		if errors.Is(err, webapp.ErrSourceUnavailable) {
			return ExportReview{}, err
		}
		return ExportReview{}, fmt.Errorf("%w: %v", ErrExportInvalid, err)
	}
	archives, err := webapp.BuildDistributionArchives(pkg)
	if err != nil {
		return ExportReview{}, fmt.Errorf("%w: %v", ErrExportInvalid, err)
	}
	preparation, err := s.preparations.Create(ctx, Preparation{
		UserID: request.UserID, WorkspaceID: item.WorkspaceID, CanvasID: item.ID, ReleaseID: release.ID,
		PackageID: metadata.PackageID, PackageVersion: metadata.Version, PackageDigest: pkg.Digest,
		Metadata:   metadata,
		OriginKind: "export", Bundle: archives.Bundle, Source: archives.Source,
		Files: distributionInventory(pkg.Files),
	})
	if err != nil {
		return ExportReview{}, err
	}
	return exportReview(preparation, metadata, pkg, len(archives.Bundle), len(archives.Source)), nil
}

func (s *DistributionService) GetExport(ctx context.Context, userID, preparationID string) (ExportReview, error) {
	preparation, err := s.preparations.Get(ctx, userID, preparationID)
	if err != nil {
		return ExportReview{}, err
	}
	if err := s.verifyCurrentRelease(ctx, preparation); err != nil {
		return ExportReview{}, err
	}
	return preparationReview(preparation), nil
}

func (s *DistributionService) DownloadExport(ctx context.Context, userID, preparationID string, kind ExportKind) (ExportDownload, error) {
	preparation, err := s.preparations.Get(ctx, userID, preparationID)
	if err != nil {
		return ExportDownload{}, err
	}
	if err := s.verifyCurrentRelease(ctx, preparation); err != nil {
		return ExportDownload{}, err
	}
	data, err := s.preparations.Open(ctx, userID, preparationID, kind)
	if err != nil {
		return ExportDownload{}, err
	}
	extension, contentType := ".tar.gz", "application/gzip"
	if kind == ExportSource {
		extension, contentType = ".zip", "application/zip"
	} else if kind != ExportBundle {
		return ExportDownload{}, ErrExportInvalid
	}
	return ExportDownload{Data: data, Filename: preparation.PackageID + "-" + preparation.PackageVersion + extension, ContentType: contentType}, nil
}

func (s *DistributionService) CancelExport(ctx context.Context, userID, preparationID string) error {
	return s.preparations.Delete(ctx, userID, preparationID)
}

func (s *DistributionService) validateReady(userID, workspaceID, canvasID string) error {
	if s == nil || s.canvases == nil || s.releases == nil || s.artifacts == nil || s.preparations == nil ||
		strings.TrimSpace(userID) == "" || strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(canvasID) == "" {
		return ErrExportInvalid
	}
	return nil
}

func (s *DistributionService) authorizeWorkspace(ctx context.Context, workspaceID string) error {
	if s.authorize == nil {
		return nil
	}
	return s.authorize(ctx, workspaceID)
}

func (s *DistributionService) loadExportSnapshot(ctx context.Context, request ExportRequest) (*Canvas, plugininstances.Release, map[string][]byte, error) {
	item, err := s.canvases.Get(ctx, request.CanvasID)
	if err != nil || !validExportCanvas(item, request) {
		return nil, plugininstances.Release{}, nil, ErrCanvasNotFound
	}
	if !matchesExpectedRelease(item, request.ExpectedReleaseID) {
		return nil, plugininstances.Release{}, nil, ErrExportStale
	}
	release, err := s.releases.GetRelease(ctx, item.ActiveReleaseID)
	if err != nil || !validExportRelease(item, release, request.ExpectedReleaseID) {
		return nil, plugininstances.Release{}, nil, ErrExportStale
	}
	files, err := s.artifacts.ReadFiles(webapp.Artifact{Digest: release.PackageDigest, RelativePath: release.ArtifactPath, Bytes: release.ArtifactBytes, Available: true})
	if err != nil {
		return nil, plugininstances.Release{}, nil, webapp.ErrArtifactUnavailable
	}
	return item, release, cloneFiles(files), nil
}

func validExportCanvas(item *Canvas, request ExportRequest) bool {
	return item != nil && item.WorkspaceID == request.WorkspaceID &&
		item.ScopeKind == ScopeWorkspace && item.Status == StatusActive &&
		item.ActiveReleaseID != "" && item.ActiveReleaseStatus == ValidationValid
}

func matchesExpectedRelease(item *Canvas, expectedReleaseID string) bool {
	return expectedReleaseID != "" && item != nil && expectedReleaseID == item.ActiveReleaseID
}

func validExportRelease(item *Canvas, release plugininstances.Release, expectedReleaseID string) bool {
	if release.ID != expectedReleaseID {
		return false
	}
	return release.InstanceID == "" || item.PluginInstanceID == "" || release.InstanceID == item.PluginInstanceID
}

func (s *DistributionService) verifyCurrentRelease(ctx context.Context, preparation Preparation) error {
	if err := s.authorizeWorkspace(ctx, preparation.WorkspaceID); err != nil {
		return err
	}
	item, err := s.canvases.Get(ctx, preparation.CanvasID)
	if err != nil || item == nil || item.WorkspaceID != preparation.WorkspaceID || item.ActiveReleaseID != preparation.ReleaseID || item.ActiveReleaseStatus != ValidationValid || item.Status != StatusActive {
		return ErrExportStale
	}
	return nil
}

func releaseManifest(release plugininstances.Release) manifest.Manifest {
	var value manifest.Manifest
	_ = json.Unmarshal(release.ManifestJSON, &value)
	return value
}

func normalizeExportMetadata(input ExportMetadata, current manifest.Manifest) (ExportMetadata, error) {
	input = fillExportMetadata(input, current)
	if exportMetadataIncomplete(input) {
		return ExportMetadata{}, fmt.Errorf("%w: package identity, description, author, license, and compatibility are required", ErrExportInvalid)
	}
	return input, nil
}

func fillExportMetadata(input ExportMetadata, current manifest.Manifest) ExportMetadata {
	if input.PackageID == "" {
		input.PackageID = current.ID
	}
	if input.Version == "" {
		input.Version = current.Version
	}
	if input.DisplayName == "" {
		input.DisplayName = current.DisplayName
	}
	if input.Description == "" {
		input.Description = current.Description
	}
	if input.Author == "" {
		input.Author = current.Author
	}
	if input.License == "" && current.Distribution != nil {
		input.License = current.Distribution.License
	}
	if input.SourceMode == "" {
		input.SourceMode = manifest.SourceModeStatic
		if current.Distribution != nil {
			input.SourceMode = current.Distribution.SourceMode
		}
	}
	if input.MinKandevVersion == "" {
		input.MinKandevVersion = current.MinKandevVersion
	}
	if input.RepoURL == "" {
		input.RepoURL = current.RepoURL
	}
	return input
}

func exportMetadataIncomplete(input ExportMetadata) bool {
	return input.PackageID == "" || input.Version == "" || input.DisplayName == "" ||
		input.Description == "" || input.Author == "" || input.License == "" || input.MinKandevVersion == ""
}

func exportManifest(data json.RawMessage, metadata ExportMetadata) (*manifest.Manifest, error) {
	var value manifest.Manifest
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("%w: release manifest is invalid", ErrExportInvalid)
	}
	value.ID, value.Version = metadata.PackageID, metadata.Version
	value.DisplayName, value.Description, value.Author = metadata.DisplayName, metadata.Description, metadata.Author
	value.MinKandevVersion, value.RepoURL = metadata.MinKandevVersion, metadata.RepoURL
	value.Distribution = &manifest.Distribution{SchemaVersion: manifest.DistributionSchemaVersion, Kind: manifest.DistributionKindCanvas, License: metadata.License, SourceMode: metadata.SourceMode}
	return &value, nil
}

func cloneFiles(input map[string][]byte) map[string][]byte {
	output := make(map[string][]byte, len(input))
	for name, data := range input {
		output[name] = append([]byte(nil), data...)
	}
	return output
}

func distributionInventory(files map[string][]byte) []ExportFile {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]ExportFile, 0, len(names))
	for _, name := range names {
		result = append(result, ExportFile{Path: name, Bytes: int64(len(files[name]))})
	}
	return result
}

func exportReview(preparation Preparation, metadata ExportMetadata, pkg *webapp.Package, bundleBytes, sourceBytes int) ExportReview {
	review := preparationReview(preparation)
	review.Metadata, review.Digest, review.Files = metadata, pkg.Digest, distributionInventory(pkg.Files)
	review.BundleBytes, review.SourceBytes = int64(bundleBytes), int64(sourceBytes)
	review.BundleDownload = "/api/v1/canvases/exports/" + preparation.ID + "/bundle"
	review.SourceDownload = "/api/v1/canvases/exports/" + preparation.ID + "/source"
	return review
}

func preparationReview(preparation Preparation) ExportReview {
	metadata := preparation.Metadata
	if metadata.PackageID == "" {
		metadata.PackageID, metadata.Version = preparation.PackageID, preparation.PackageVersion
	}
	return ExportReview{PreparationID: preparation.ID, CanvasID: preparation.CanvasID, WorkspaceID: preparation.WorkspaceID, ReleaseID: preparation.ReleaseID, Metadata: metadata, Digest: preparation.PackageDigest, Files: append([]ExportFile(nil), preparation.Files...), ExpiresAt: preparation.ExpiresAt.UTC().Format("2006-01-02T15:04:05.000Z07:00")}
}

func (s *DistributionService) preparationsCount() int {
	if s == nil || s.preparations == nil {
		return 0
	}
	s.preparations.mu.Lock()
	defer s.preparations.mu.Unlock()
	return len(s.preparations.items)
}
