package canvas

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/webapp"
)

var (
	ErrInstallInvalid         = errors.New("invalid canvas installation")
	ErrPackageDigestMismatch  = errors.New("canvas package digest mismatch")
	ErrInstallNotApproved     = errors.New("canvas installation approval is required")
	ErrInstallUnavailable     = errors.New("canvas installation is unavailable")
	ErrInstallReceiptNotFound = errors.New("canvas installation receipt not found")
	ErrInstallIncompatible    = errors.New("canvas package requires a newer Kandev version")
)

const (
	installOriginUpload   = "upload"
	installOriginURL      = "url"
	installOriginRegistry = "registry"
)

type ArtifactWriter interface {
	Put(*webapp.Package) (webapp.Artifact, error)
}

type artifactWriterWithCreated interface {
	PutWithCreated(*webapp.Package) (webapp.Artifact, bool, error)
}

type artifactRemover interface {
	Remove(webapp.Artifact) error
}

type CanvasInstaller interface {
	CanvasReader
	InstallCanvasPackage(context.Context, InstallCanvasPackageRequest) (*InstallResult, error)
}

// InstallCanvasPackageRequest is the trusted, already-inspected input for the
// single database transaction used by the production canvas service.
type InstallCanvasPackageRequest struct {
	WorkspaceID     string
	Title           string
	Package         *webapp.Package
	Artifact        webapp.Artifact
	SourceActorKind string
	SourceUserID    string
	Approved        bool
	Receipt         InstallReceipt
}

type InstallReceiptStore interface {
	GetInstallReceipt(context.Context, string, string) (InstallReceipt, error)
	CreateInstallReceipt(context.Context, InstallReceipt) error
}

type InstallConfirmation struct {
	ExpectedDigest string `json:"expected_digest"`
	Approved       bool   `json:"approved"`
}

type InstallReceipt struct {
	PreparationID string    `json:"preparation_id"`
	UserID        string    `json:"user_id"`
	CanvasID      string    `json:"canvas_id"`
	WorkspaceID   string    `json:"workspace_id"`
	PackageID     string    `json:"package_id"`
	Version       string    `json:"version"`
	Digest        string    `json:"sha256"`
	SourceID      string    `json:"source_id,omitempty"`
	RepositoryURL string    `json:"repository_url,omitempty"`
	OriginKind    string    `json:"origin_kind"`
	CreatedAt     time.Time `json:"created_at"`
}

type InstallResult struct {
	Canvas  *Canvas        `json:"canvas"`
	Receipt InstallReceipt `json:"receipt"`
}

type InstallRequest struct {
	UserID                string
	WorkspaceID           string
	OriginKind            string
	SourceID              string
	RepositoryURL         string
	PackageID             string
	ExpectedVersion       string
	ExpectedDigest        string
	ExpectedArchiveDigest string
	Bundle                []byte
}

// GetInstallReview returns the authoritative inspected metadata for a staged
// install. It reopens the staged bytes so permissions are never reconstructed
// from browser-supplied fields.
func (s *DistributionService) GetInstallReview(ctx context.Context, userID, preparationID string) (InstallReview, error) {
	if s == nil || s.preparations == nil || strings.TrimSpace(userID) == "" || strings.TrimSpace(preparationID) == "" {
		return InstallReview{}, ErrInstallInvalid
	}
	preparation, err := s.preparations.Get(ctx, userID, preparationID)
	if err != nil {
		return InstallReview{}, err
	}
	if err := s.authorizeWorkspace(ctx, preparation.WorkspaceID); err != nil {
		return InstallReview{}, err
	}
	bundle, err := s.preparations.Open(ctx, userID, preparationID, ExportBundle)
	if err != nil {
		return InstallReview{}, err
	}
	pkg, err := webapp.ValidateDistributionPackage(bytes.NewReader(bundle))
	if err != nil {
		return InstallReview{}, fmt.Errorf("%w: %v", ErrInstallInvalid, err)
	}
	if pkg.Digest != preparation.PackageDigest {
		return InstallReview{}, ErrPackageDigestMismatch
	}
	return installReview(preparation, pkg), nil
}

// CancelInstall removes an unconfirmed install review.
func (s *DistributionService) CancelInstall(ctx context.Context, userID, preparationID string) error {
	if s == nil || s.preparations == nil {
		return ErrInstallInvalid
	}
	return s.preparations.Delete(ctx, userID, preparationID)
}

type InstallReview struct {
	PreparationID     string            `json:"preparation_id"`
	WorkspaceID       string            `json:"workspace_id"`
	Metadata          InstallMetadata   `json:"metadata"`
	Digest            string            `json:"sha256"`
	ArchiveDigest     string            `json:"archive_sha256"`
	PermissionSummary PermissionSummary `json:"permissions"`
	OriginKind        string            `json:"origin_kind"`
	SourceID          string            `json:"source_id,omitempty"`
	RepositoryURL     string            `json:"repository_url,omitempty"`
	ExpiresAt         string            `json:"expires_at"`
}

type InstallMetadata struct {
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

func (s *DistributionService) PrepareInstall(ctx context.Context, request InstallRequest) (InstallReview, error) {
	if err := s.validateInstallRequest(ctx, request); err != nil {
		return InstallReview{}, err
	}
	origin, err := normalizeInstallOrigin(request.OriginKind)
	if err != nil {
		return InstallReview{}, err
	}
	pkg, archive, err := inspectInstallBundle(request, origin)
	if err != nil {
		return InstallReview{}, err
	}
	if err := s.validateInstallCompatibility(pkg); err != nil {
		return InstallReview{}, err
	}
	return s.stageInstall(ctx, request, origin, archive, pkg)
}

func (s *DistributionService) validateInstallCompatibility(pkg *webapp.Package) error {
	if pkg == nil || pkg.Manifest == nil {
		return ErrInstallInvalid
	}
	s.installMu.Lock()
	runningVersion := s.kandevVersion
	s.installMu.Unlock()
	if err := manifest.CheckMinimumKandevVersion(pkg.Manifest.MinKandevVersion, runningVersion); err != nil {
		return fmt.Errorf("%w: %v", ErrInstallIncompatible, err)
	}
	return nil
}

func (s *DistributionService) validateInstallRequest(ctx context.Context, request InstallRequest) error {
	if s == nil || s.preparations == nil || strings.TrimSpace(request.UserID) == "" || strings.TrimSpace(request.WorkspaceID) == "" || len(request.Bundle) == 0 {
		return ErrInstallInvalid
	}
	return s.authorizeWorkspace(ctx, request.WorkspaceID)
}

func normalizeInstallOrigin(origin string) (string, error) {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return installOriginUpload, nil
	}
	if origin != installOriginUpload && origin != installOriginURL && origin != installOriginRegistry {
		return "", ErrInstallInvalid
	}
	return origin, nil
}

func inspectInstallBundle(request InstallRequest, origin string) (*webapp.Package, string, error) {
	pkg, err := webapp.ValidateDistributionPackage(bytes.NewReader(request.Bundle))
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrInstallInvalid, err)
	}
	archive := archiveDigest(request.Bundle)
	if err := validateInstallExpectations(request, origin, pkg, archive); err != nil {
		return nil, "", err
	}
	return pkg, archive, nil
}

func validateInstallExpectations(request InstallRequest, origin string, pkg *webapp.Package, archive string) error {
	if request.PackageID != "" && request.PackageID != pkg.Manifest.ID {
		return ErrPackageDigestMismatch
	}
	if request.ExpectedVersion != "" && request.ExpectedVersion != pkg.Manifest.Version {
		return ErrPackageDigestMismatch
	}
	if request.ExpectedDigest != "" && request.ExpectedDigest != pkg.Digest {
		return ErrPackageDigestMismatch
	}
	if request.ExpectedArchiveDigest != "" && request.ExpectedArchiveDigest != archive {
		return ErrPackageDigestMismatch
	}
	return nil
}

func (s *DistributionService) stageInstall(ctx context.Context, request InstallRequest, origin, archive string, pkg *webapp.Package) (InstallReview, error) {
	metadata := installMetadata(pkg.Manifest)
	repositoryURL := sanitizeRepositoryURL(request.RepositoryURL)
	preparation, err := s.preparations.Create(ctx, Preparation{
		UserID: request.UserID, WorkspaceID: request.WorkspaceID, PackageID: pkg.Manifest.ID,
		PackageVersion: pkg.Manifest.Version, PackageDigest: pkg.Digest, PackageArchiveDigest: archive, OriginKind: origin,
		SourceID: request.SourceID, RepositoryURL: repositoryURL,
		Metadata: ExportMetadata{PackageID: pkg.Manifest.ID, Version: pkg.Manifest.Version, DisplayName: pkg.Manifest.DisplayName, Description: pkg.Manifest.Description, Author: pkg.Manifest.Author, License: pkg.Manifest.Distribution.License, SourceMode: pkg.Manifest.Distribution.SourceMode, MinKandevVersion: pkg.Manifest.MinKandevVersion, RepoURL: pkg.Manifest.RepoURL},
		Bundle:   append([]byte(nil), request.Bundle...), Files: distributionInventory(pkg.Files),
	})
	if err != nil {
		return InstallReview{}, err
	}
	return InstallReview{PreparationID: preparation.ID, WorkspaceID: request.WorkspaceID, Metadata: metadata, Digest: pkg.Digest, ArchiveDigest: archive, PermissionSummary: ManifestPermissions(pkg.Manifest), OriginKind: origin, SourceID: request.SourceID, RepositoryURL: repositoryURL, ExpiresAt: preparation.ExpiresAt.UTC().Format("2006-01-02T15:04:05.000Z07:00")}, nil
}

// PrepareInstallFromURL downloads one bounded HTTPS bundle and routes the
// resulting bytes through the same inspection path as uploads.
func (s *DistributionService) PrepareInstallFromURL(ctx context.Context, request InstallRequest, rawURL string) (InstallReview, error) {
	if s == nil || strings.TrimSpace(request.UserID) == "" || strings.TrimSpace(request.WorkspaceID) == "" {
		return InstallReview{}, ErrInstallInvalid
	}
	if err := s.authorizeWorkspace(ctx, request.WorkspaceID); err != nil {
		return InstallReview{}, err
	}
	bundle, err := s.downloadInstallBundle(ctx, rawURL)
	if err != nil {
		return InstallReview{}, err
	}
	request.Bundle = bundle
	if strings.TrimSpace(request.OriginKind) == "" {
		request.OriginKind = installOriginURL
	}
	if strings.TrimSpace(request.RepositoryURL) == "" {
		request.RepositoryURL = sanitizeRepositoryURL(rawURL)
	}
	return s.PrepareInstall(ctx, request)
}

// PrepareInstallFromCatalog resolves and downloads the exact server-selected
// canvas listing before entering the normal inspected-byte review path.
func (s *DistributionService) PrepareInstallFromCatalog(ctx context.Context, request InstallRequest) (InstallReview, error) {
	if s == nil || strings.TrimSpace(request.UserID) == "" || strings.TrimSpace(request.WorkspaceID) == "" || strings.TrimSpace(request.SourceID) == "" || strings.TrimSpace(request.PackageID) == "" || strings.TrimSpace(request.ExpectedVersion) == "" || strings.TrimSpace(request.ExpectedArchiveDigest) == "" {
		return InstallReview{}, ErrInstallInvalid
	}
	if err := s.authorizeWorkspace(ctx, request.WorkspaceID); err != nil {
		return InstallReview{}, err
	}
	s.installMu.Lock()
	resolver := s.catalog
	s.installMu.Unlock()
	if resolver == nil {
		return InstallReview{}, ErrInstallUnavailable
	}
	packageURL, repositoryURL, err := resolver.ResolveCanvasPackage(ctx, request.SourceID, request.PackageID, request.ExpectedVersion, request.ExpectedArchiveDigest)
	if err != nil {
		return InstallReview{}, err
	}
	request.OriginKind = installOriginRegistry
	request.RepositoryURL = repositoryURL
	request.ExpectedDigest = ""
	return s.PrepareInstallFromURL(ctx, request, packageURL)
}

func (s *DistributionService) ConfirmInstall(ctx context.Context, userID, preparationID string, confirmation InstallConfirmation) (InstallResult, error) {
	if s == nil || s.preparations == nil || strings.TrimSpace(userID) == "" || strings.TrimSpace(preparationID) == "" {
		return InstallResult{}, ErrInstallInvalid
	}
	if !confirmation.Approved {
		return InstallResult{}, ErrInstallNotApproved
	}
	if result, ok, err := s.lookupReceipt(ctx, userID, preparationID); err != nil {
		return InstallResult{}, err
	} else if ok {
		return s.authorizeInstallReceipt(ctx, result)
	}
	s.installMu.Lock()
	defer s.installMu.Unlock()
	if result, ok, err := s.lookupReceiptLocked(ctx, userID, preparationID); err != nil {
		return InstallResult{}, err
	} else if ok {
		return s.authorizeInstallReceipt(ctx, result)
	}
	preparation, err := s.preparations.Get(ctx, userID, preparationID)
	if err != nil {
		return InstallResult{}, err
	}
	if err := s.authorizeWorkspace(ctx, preparation.WorkspaceID); err != nil {
		return InstallResult{}, err
	}
	pkg, err := s.openInstallPackage(ctx, userID, preparation, confirmation.ExpectedDigest)
	if err != nil {
		return InstallResult{}, err
	}
	receipt := InstallReceipt{PreparationID: preparation.ID, UserID: userID, WorkspaceID: preparation.WorkspaceID, PackageID: preparation.PackageID, Version: preparation.PackageVersion, Digest: preparation.PackageDigest, SourceID: preparation.SourceID, RepositoryURL: sanitizeRepositoryURL(preparation.RepositoryURL), OriginKind: preparation.OriginKind, CreatedAt: time.Now().UTC()}
	return s.installPreparedPackage(ctx, userID, preparation, pkg, receipt)
}

func (s *DistributionService) authorizeInstallReceipt(ctx context.Context, result InstallResult) (InstallResult, error) {
	if err := s.authorizeWorkspace(ctx, result.Receipt.WorkspaceID); err != nil {
		return InstallResult{}, err
	}
	return result, nil
}

func (s *DistributionService) installPreparedPackage(ctx context.Context, userID string, preparation Preparation, pkg *webapp.Package, receipt InstallReceipt) (InstallResult, error) {
	installer, ok := s.canvases.(CanvasInstaller)
	if !ok {
		return InstallResult{}, ErrInstallUnavailable
	}
	if err := s.authorizeWorkspace(ctx, preparation.WorkspaceID); err != nil {
		return InstallResult{}, err
	}
	writer, ok := s.artifacts.(ArtifactWriter)
	if !ok {
		return InstallResult{}, ErrInstallUnavailable
	}
	artifact, created, err := putInstallArtifact(writer, pkg)
	if err != nil {
		return InstallResult{}, err
	}
	result, err := installer.InstallCanvasPackage(ctx, InstallCanvasPackageRequest{WorkspaceID: preparation.WorkspaceID, Title: pkg.Manifest.DisplayName, Package: pkg, Artifact: artifact, SourceActorKind: "canvas-install:" + preparation.OriginKind, SourceUserID: userID, Approved: true, Receipt: receipt})
	if err != nil {
		return s.handleInstallFailure(ctx, userID, preparation.ID, writer, artifact, created, result, err)
	}
	if !validInstallResult(result) {
		return InstallResult{}, ErrInstallUnavailable
	}
	if err := s.cacheInstallReceipt(ctx, userID, preparation.ID, result.Receipt); err != nil {
		return InstallResult{}, err
	}
	return *result, nil
}

func (s *DistributionService) handleInstallFailure(ctx context.Context, userID, preparationID string, writer ArtifactWriter, artifact webapp.Artifact, created bool, result *InstallResult, installErr error) (InstallResult, error) {
	existing, found, lookupErr := s.lookupReceiptLocked(ctx, userID, preparationID)
	if lookupErr != nil {
		return InstallResult{}, errors.Join(installErr, lookupErr)
	}
	if found {
		return s.authorizeInstallReceipt(ctx, existing)
	}
	if !created || validInstallResult(result) {
		return InstallResult{}, installErr
	}
	remover, ok := writer.(artifactRemover)
	if !ok {
		return InstallResult{}, installErr
	}
	if cleanupErr := remover.Remove(artifact); cleanupErr != nil {
		return InstallResult{}, errors.Join(installErr, cleanupErr)
	}
	return InstallResult{}, installErr
}

func validInstallResult(result *InstallResult) bool {
	return result != nil && result.Canvas != nil && result.Receipt.CanvasID != ""
}

func (s *DistributionService) openInstallPackage(ctx context.Context, userID string, preparation Preparation, expectedDigest string) (*webapp.Package, error) {
	if expectedDigest == "" || (expectedDigest != preparation.PackageDigest && expectedDigest != preparation.PackageArchiveDigest) {
		return nil, ErrPackageDigestMismatch
	}
	bundle, err := s.preparations.Open(ctx, userID, preparation.ID, ExportBundle)
	if err != nil {
		return nil, err
	}
	pkg, err := webapp.ValidateDistributionPackage(bytes.NewReader(bundle))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInstallInvalid, err)
	}
	if pkg.Digest != preparation.PackageDigest {
		return nil, ErrPackageDigestMismatch
	}
	return pkg, nil
}

func (s *DistributionService) cacheInstallReceipt(ctx context.Context, userID, preparationID string, receipt InstallReceipt) error {
	if s.receipts == nil {
		s.receipts = make(map[string]InstallReceipt)
	}
	s.receipts[preparationID] = receipt
	return s.preparations.Delete(ctx, userID, preparationID)
}

func (s *DistributionService) lookupReceipt(ctx context.Context, userID, preparationID string) (InstallResult, bool, error) {
	s.installMu.Lock()
	defer s.installMu.Unlock()
	return s.lookupReceiptLocked(ctx, userID, preparationID)
}

func (s *DistributionService) lookupReceiptLocked(ctx context.Context, userID, preparationID string) (InstallResult, bool, error) {
	receipt, ok := s.receipts[preparationID]
	if !ok && s.receiptStore != nil {
		stored, err := s.receiptStore.GetInstallReceipt(ctx, preparationID, userID)
		if err == nil {
			receipt, ok = stored, true
			s.receipts[preparationID] = stored
		} else if !errors.Is(err, ErrInstallReceiptNotFound) {
			return InstallResult{}, false, err
		}
	}
	if !ok || receipt.UserID != userID {
		return InstallResult{}, false, nil
	}
	item, err := s.canvases.Get(ctx, receipt.CanvasID)
	if errors.Is(err, ErrCanvasNotFound) || item == nil || item.WorkspaceID != receipt.WorkspaceID {
		return InstallResult{}, false, nil
	}
	if err != nil {
		return InstallResult{}, false, err
	}
	return InstallResult{Canvas: item, Receipt: receipt}, true, nil
}

func putInstallArtifact(writer ArtifactWriter, pkg *webapp.Package) (webapp.Artifact, bool, error) {
	if withCreated, ok := writer.(artifactWriterWithCreated); ok {
		return withCreated.PutWithCreated(pkg)
	}
	artifact, err := writer.Put(pkg)
	return artifact, false, err
}

func installMetadata(m *manifest.Manifest) InstallMetadata {
	if m == nil || m.Distribution == nil {
		return InstallMetadata{}
	}
	return InstallMetadata{PackageID: m.ID, Version: m.Version, DisplayName: m.DisplayName, Description: m.Description, Author: m.Author, License: m.Distribution.License, SourceMode: m.Distribution.SourceMode, MinKandevVersion: m.MinKandevVersion, RepoURL: m.RepoURL}
}

func sanitizeRepositoryURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	// Provenance is public metadata only. Queries and fragments can contain
	// signed-download credentials, so they are never persisted.
	if index := strings.IndexAny(value, "?#"); index >= 0 {
		value = value[:index]
	}
	return value
}

func installReview(preparation Preparation, pkg *webapp.Package) InstallReview {
	metadata := installMetadata(pkg.Manifest)
	return InstallReview{
		PreparationID: preparation.ID, WorkspaceID: preparation.WorkspaceID,
		Metadata: metadata, Digest: pkg.Digest, ArchiveDigest: preparation.PackageArchiveDigest,
		PermissionSummary: ManifestPermissions(pkg.Manifest), OriginKind: preparation.OriginKind,
		SourceID: preparation.SourceID, RepositoryURL: sanitizeRepositoryURL(preparation.RepositoryURL),
		ExpiresAt: preparation.ExpiresAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
	}
}
