package plugins

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/plugins/marketplace"
	"github.com/kandev/kandev/internal/plugins/pkgtar"
	"github.com/kandev/kandev/internal/plugins/provenance"
	"github.com/kandev/kandev/internal/plugins/store"
)

// VerifyInstalledPublisher verifies the exact bytes of an installed native
// version against the matching release in the canonical official catalog.
// It changes only host-owned publisher provenance; runtime state, package
// files, configuration, approvals, and the original origin remain intact.
func (s *Service) VerifyInstalledPublisher(ctx context.Context, id, expectedInstallationID, expectedVersion string) (*store.Record, error) {
	initial, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if !matchesExpectedInstalledIdentity(initial, id, expectedInstallationID, expectedVersion) {
		return nil, publisherError(PublisherCodeInstalledChanged, ErrInstalledPackageChanged)
	}
	if hasVerifiedPublisher(initial) {
		return initial, nil
	}

	resolution, inspection, digest, err := s.loadPublisherVerificationPackage(ctx, id, initial.Version)
	if err != nil {
		return nil, err
	}
	return s.persistInstalledPublisherVerification(id, initial, resolution, inspection.Files, inspection.Modes, digest)
}

func matchesExpectedInstalledIdentity(record *store.Record, id, installationID, version string) bool {
	return record != nil && record.ID == id && record.InstallationID == installationID && record.Version == version
}

func hasVerifiedPublisher(record *store.Record) bool {
	return record != nil && record.PublisherProvenance != nil &&
		record.PublisherProvenance.Identity().Status == provenance.StatusVerified
}

func (s *Service) loadPublisherVerificationPackage(ctx context.Context, id, version string) (*marketplace.PluginPackageResolution, *pkgtar.Inspection, string, error) {
	m := s.Marketplace()
	if m == nil {
		return nil, nil, "", publisherError(PublisherCodeCatalogUnavailable, marketplace.ErrCatalogUnavailable)
	}
	resolution, err := m.ResolveOfficialPluginPackage(ctx, id, version)
	if err != nil {
		return nil, nil, "", mapPublisherResolutionError(err)
	}
	if !hasVerifiedResolutionEvidence(resolution) {
		return nil, nil, "", publisherError(PublisherCodeEvidenceUnavailable, ErrPublisherEvidenceUnavailable)
	}
	data, digest, err := s.downloadPackage(ctx, resolution.PackageURL)
	if err != nil {
		return nil, nil, "", publisherError(PublisherCodeCatalogUnavailable, err)
	}
	if !strings.EqualFold(digest, resolution.Entry.PackageSHA256) {
		return nil, nil, "", publisherError(PublisherCodeEvidenceUnavailable, ErrPublisherEvidenceUnavailable)
	}
	inspection, err := pkgtar.InspectPackage(bytes.NewReader(data))
	if err != nil {
		return nil, nil, "", publisherError(PublisherCodeEvidenceUnavailable, err)
	}
	if inspection.Manifest.ID != id || inspection.Manifest.Version != version {
		return nil, nil, "", publisherError(PublisherCodePackageIdentityMismatch, ErrPackageIdentityMismatch)
	}
	return resolution, inspection, digest, nil
}

func hasVerifiedResolutionEvidence(resolution *marketplace.PluginPackageResolution) bool {
	return resolution != nil && resolution.Publisher != nil &&
		resolution.Publisher.Status == provenance.StatusVerified &&
		resolution.Provenance != nil && resolution.Provenance.Publisher != nil
}

func (s *Service) persistInstalledPublisherVerification(id string, initial *store.Record, resolution *marketplace.PluginPackageResolution, expected map[string][]byte, expectedModes map[string]os.FileMode, digest string) (*store.Record, error) {

	lock := s.lifecycleLocks.lockFor(id)
	lock.Lock()
	defer lock.Unlock()
	dispatchLock := s.dispatchLocks.lockFor(id)
	dispatchLock.Lock()
	defer dispatchLock.Unlock()
	current, ok := s.registry.Get(id)
	if !ok || !sameInstalledIdentity(initial, current) {
		return nil, publisherError(PublisherCodeInstalledChanged, ErrInstalledPackageChanged)
	}
	restart, err := s.pauseRuntimeForPublisherVerification(current)
	if err != nil {
		return nil, err
	}
	verificationErr := compareInstalledPackage(current.InstallPath, expected, expectedModes)
	if verificationErr == nil {
		verificationErr = s.saveInstalledPublisherVerification(current, resolution, digest)
	}
	if restartErr := restart(); verificationErr == nil && restartErr != nil {
		verificationErr = publisherError(PublisherCodeVerificationFailed, restartErr)
	}
	if verificationErr != nil {
		return nil, verificationErr
	}
	return current, nil
}

func (s *Service) saveInstalledPublisherVerification(current *store.Record, resolution *marketplace.PluginPackageResolution, digest string) error {
	updated := installedPublisherProvenance(current, resolution, digest)
	if err := updated.Validate(); err != nil {
		return publisherError(PublisherCodeVerificationFailed, err)
	}
	current.PublisherProvenance = updated
	current.PublisherIdentity = updated.Identity()
	if err := s.store.Save(current); err != nil {
		return publisherError(PublisherCodeVerificationFailed, fmt.Errorf("persist publisher verification: %w", err))
	}
	s.registry.Add(current)
	return nil
}

func (s *Service) pauseRuntimeForPublisherVerification(current *store.Record) (func() error, error) {
	if s.runtime == nil || !s.runtime.Running(current.ID) {
		return func() error { return nil }, nil
	}
	s.runtime.Stop(current.ID)
	return func() error {
		ctx, cancel := context.WithTimeout(context.Background(), activateStartTimeout)
		defer cancel()
		if err := s.runtime.Start(ctx, current, s.hostForPlugin); err != nil {
			if setErr := s.setStatusAndDiagnostic(current.ID, StatusError, err, true); setErr != nil {
				return fmt.Errorf("restart plugin %q after publisher verification: %w (persist status: %v)", current.ID, err, setErr)
			}
			return fmt.Errorf("restart plugin %q after publisher verification: %w", current.ID, err)
		}
		return nil
	}, nil
}

func installedPublisherProvenance(current *store.Record, resolution *marketplace.PluginPackageResolution, digest string) *provenance.InstallationProvenance {
	updated := current.PublisherProvenance.Clone()
	if updated == nil {
		updated = &provenance.InstallationProvenance{Origin: provenance.OriginUnknown}
	}
	if updated.Origin == "" {
		updated.Origin = provenance.OriginUnknown
	}
	updated.PackageID = current.ID
	updated.Version = current.Version
	updated.PackageSHA256 = strings.ToLower(digest)
	updated.Publisher = resolution.Provenance.Publisher
	now := time.Now().UTC()
	updated.VerifiedAt = &now
	updated.VerificationMethod = provenance.VerificationInstalledFiles
	updated.MatchedSourceID = resolution.Source.ID
	updated.MatchedSourceURL = resolution.Source.URL
	updated.SanitizePublicURLs()
	return updated
}

func mapPublisherResolutionError(err error) error {
	switch {
	case errors.Is(err, marketplace.ErrCatalogUnavailable):
		return publisherError(PublisherCodeCatalogUnavailable, err)
	case errors.Is(err, marketplace.ErrPluginListingStale), errors.Is(err, marketplace.ErrPluginListingNotFound):
		return publisherError(PublisherCodeEvidenceUnavailable, ErrPublisherEvidenceUnavailable)
	default:
		return publisherError(PublisherCodeEvidenceUnavailable, err)
	}
}

func sameInstalledIdentity(a, b *store.Record) bool {
	if a == nil || b == nil {
		return false
	}
	return a.ID == b.ID && a.InstallationID == b.InstallationID && a.Version == b.Version && a.InstallPath == b.InstallPath && reflect.DeepEqual(a.PublisherProvenance, b.PublisherProvenance)
}

// compareInstalledPackage compares every regular file in an extracted
// version directory with the validated archive inventory. It deliberately
// ignores timestamps and empty directories, while rejecting links and special
// files so a path cannot resolve outside the installed version.
func compareInstalledPackage(root string, expected map[string][]byte, expectedModes map[string]os.FileMode) error {
	return compareInstalledPackageWithAfterReadHook(root, expected, expectedModes, nil)
}

func compareInstalledPackageWithAfterReadHook(root string, expected map[string][]byte, expectedModes map[string]os.FileMode, afterRead func(string)) error {
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return publisherError(PublisherCodeInstalledUnreadable, fmt.Errorf("%w: %v", ErrInstalledPackageUnreadable, err))
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return publisherError(PublisherCodeInstalledMismatch, ErrInstalledPackageMismatch)
	}
	seen := make(map[string]bool, len(expected))
	walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return publisherError(PublisherCodeInstalledUnreadable, fmt.Errorf("%w: %v", ErrInstalledPackageUnreadable, walkErr))
		}
		if path == root {
			return nil
		}
		if err := validateInstalledEntry(entry); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := installedRelativePath(root, path)
		if err != nil {
			return err
		}
		want, ok := expected[rel]
		if !ok {
			return publisherError(PublisherCodeInstalledMismatch, ErrInstalledPackageMismatch)
		}
		seen[rel] = true
		wantMode, modeRequired := expectedModes[rel]
		return compareInstalledFile(path, want, wantMode, modeRequired, afterRead)
	})
	if walkErr != nil {
		return walkErr
	}
	if len(seen) != len(expected) {
		return publisherError(PublisherCodeInstalledMismatch, ErrInstalledPackageMismatch)
	}
	for name := range expected {
		if !seen[name] {
			return publisherError(PublisherCodeInstalledMismatch, ErrInstalledPackageMismatch)
		}
	}
	return nil
}

func validateInstalledEntry(entry os.DirEntry) error {
	mode := entry.Type()
	if mode&os.ModeSymlink != 0 || mode&os.ModeNamedPipe != 0 || mode&os.ModeSocket != 0 || mode&os.ModeDevice != 0 {
		return publisherError(PublisherCodeInstalledMismatch, ErrInstalledPackageMismatch)
	}
	if entry.IsDir() || mode.IsRegular() {
		return nil
	}
	return publisherError(PublisherCodeInstalledMismatch, ErrInstalledPackageMismatch)
}

func installedRelativePath(root, path string) (string, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || filepath.IsAbs(rel) {
		return "", publisherError(PublisherCodeInstalledMismatch, ErrInstalledPackageMismatch)
	}
	return filepath.ToSlash(rel), nil
}

func compareInstalledFile(path string, want []byte, wantMode os.FileMode, modeRequired bool, afterRead func(string)) error {
	before, err := statInstalledFile(path, wantMode, modeRequired, int64(len(want)))
	if err != nil {
		return err
	}
	data, err := readInstalledFile(path, before, len(want))
	if err != nil {
		return err
	}
	if !bytes.Equal(data, want) {
		return publisherError(PublisherCodeInstalledMismatch, ErrInstalledPackageMismatch)
	}
	if afterRead != nil {
		afterRead(path)
	}
	after, err := os.Lstat(path)
	if err != nil || !sameInstalledPathState(before, after) {
		return publisherError(PublisherCodeInstalledChanged, ErrInstalledPackageChanged)
	}
	return nil
}

func statInstalledFile(path string, wantMode os.FileMode, modeRequired bool, wantSize int64) (os.FileInfo, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, publisherError(PublisherCodeInstalledUnreadable, fmt.Errorf("%w: %v", ErrInstalledPackageUnreadable, err))
	}
	if !before.Mode().IsRegular() || before.Mode().Perm()&0444 == 0 {
		return nil, publisherError(PublisherCodeInstalledUnreadable, ErrInstalledPackageUnreadable)
	}
	if modeRequired && before.Mode().Perm() != wantMode.Perm() {
		return nil, publisherError(PublisherCodeInstalledMismatch, ErrInstalledPackageMismatch)
	}
	if before.Size() != wantSize {
		return nil, publisherError(PublisherCodeInstalledMismatch, ErrInstalledPackageMismatch)
	}
	return before, nil
}

func readInstalledFile(path string, before os.FileInfo, wantSize int) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, publisherError(PublisherCodeInstalledUnreadable, fmt.Errorf("%w: %v", ErrInstalledPackageUnreadable, err))
	}
	if err := validateOpenedInstalledFile(path, before, file); err != nil {
		_ = file.Close()
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, int64(wantSize)+1))
	openedAfter, statErr := file.Stat()
	pathAfter, pathErr := os.Lstat(path)
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return nil, publisherError(PublisherCodeInstalledUnreadable, ErrInstalledPackageUnreadable)
	}
	if statErr != nil || pathErr != nil || !sameInstalledFileState(before, openedAfter, pathAfter) {
		return nil, publisherError(PublisherCodeInstalledChanged, ErrInstalledPackageChanged)
	}
	return data, nil
}

func validateOpenedInstalledFile(path string, before os.FileInfo, file *os.File) error {
	opened, err := file.Stat()
	if err != nil {
		return publisherError(PublisherCodeInstalledChanged, ErrInstalledPackageChanged)
	}
	current, err := os.Lstat(path)
	if err != nil || !sameInstalledFileState(before, opened, current) {
		return publisherError(PublisherCodeInstalledChanged, ErrInstalledPackageChanged)
	}
	return nil
}

func sameInstalledFileState(before, opened, path os.FileInfo) bool {
	return opened != nil && sameInstalledPathState(before, path) &&
		opened.Mode().IsRegular() && os.SameFile(before, opened) &&
		before.Size() == opened.Size() && before.Mode() == opened.Mode() && before.ModTime().Equal(opened.ModTime())
}

func sameInstalledPathState(before, after os.FileInfo) bool {
	return after != nil && after.Mode()&os.ModeSymlink == 0 && os.SameFile(before, after) &&
		before.Size() == after.Size() && before.Mode() == after.Mode() && before.ModTime().Equal(after.ModTime())
}
