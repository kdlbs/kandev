package plugins

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
)

// tarballSuffix identifies a dropped plugin package file directly under the
// plugins directory (see scanTarballs).
const tarballSuffix = ".tar.gz"

// Sync scans the plugins directory for filesystem-level changes an operator
// made outside the install/enable/disable/uninstall API — directory
// sideloads and dropped tarballs — and reconciles the registry with what
// actually exists on disk, per docs/specs/plugins/spec.md ("Filesystem
// sideloading & sync"):
//
//  1. Dir sideloads: an extracted <id>/<version>/manifest.yaml with no
//     existing {id}.yml record is registered StatusDisabled (never
//     auto-spawned — sideloads are unverified).
//  2. Dropped tarballs: any *.tar.gz file sitting directly in the plugins
//     directory is run through the normal verified install pipeline
//     (Service.Install); its file is deleted on success.
//  3. Missing installs: any registered record whose InstallPath no longer
//     exists on disk is stopped (if running) and marked StatusError.
//
// Individual item failures never abort the rest of the scan; they are
// collected into SyncResult.Errors. Concurrent Sync calls are serialized by
// syncMu so they cannot race each other into a double install.
func (s *Service) Sync(ctx context.Context) (*SyncResult, error) {
	return s.sync(ctx, true), nil
}

// bootScan runs the conservative subset of Sync at startup: dir sideloads
// (registered disabled, so nothing new is ever auto-spawned) and
// missing-install detection. Dropped tarballs are intentionally left alone
// at boot — installing and spawning an unverified binary as a side effect
// of starting up is out of scope; an operator triggers that via Sync (the
// UI's Sync button or POST /api/plugins/sync).
func (s *Service) bootScan(ctx context.Context) *SyncResult {
	return s.sync(ctx, false)
}

// sync is Sync's and bootScan's shared implementation, parameterized on
// whether dropped tarballs are installed this run.
func (s *Service) sync(ctx context.Context, includeTarballs bool) *SyncResult {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()

	result := &SyncResult{}
	if s.pluginsDir == "" {
		return result
	}

	added, sideloadErrs := s.scanDirSideloads()
	result.Added = added
	result.Errors = append(result.Errors, sideloadErrs...)

	if includeTarballs {
		installed, tarballErrs := s.scanTarballs(ctx)
		result.Installed = installed
		result.Errors = append(result.Errors, tarballErrs...)
	}

	result.Missing = s.scanMissingInstalls()

	s.notifyDeliverer()
	return result
}

// scanDirSideloads finds every <pluginsDir>/<id>/<version>/manifest.yaml
// tree with no existing {id}.yml record and registers it StatusDisabled.
// When more than one unregistered version dir exists for the same id, the
// lexically greatest version is chosen and the rest are reported as skipped
// via the returned errs.
func (s *Service) scanDirSideloads() (added []string, errs []SyncError) {
	entries, err := os.ReadDir(s.pluginsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, []SyncError{{Path: s.pluginsDir, Reason: err.Error()}}
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		if _, ok := s.registry.Get(id); ok {
			continue // already registered, not a sideload candidate
		}

		version, skipped, err := s.chooseSideloadVersion(id)
		if err != nil {
			errs = append(errs, SyncError{Path: filepath.Join(s.pluginsDir, id), Reason: err.Error()})
			continue
		}
		if version == "" {
			continue // no manifest.yaml found under any version dir
		}
		for _, skip := range skipped {
			errs = append(errs, SyncError{
				Path:   filepath.Join(s.pluginsDir, id, skip),
				Reason: fmt.Sprintf("skipped: version %q already selected for unregistered plugin %q", version, id),
			})
		}

		if err := s.registerSideload(id, version); err != nil {
			errs = append(errs, SyncError{Path: filepath.Join(s.pluginsDir, id, version), Reason: err.Error()})
			continue
		}
		added = append(added, id)
	}
	return added, errs
}

// chooseSideloadVersion lists id's version directories under pluginsDir and
// returns the lexically greatest one containing a manifest.yaml, plus every
// other candidate (to be reported as skipped by the caller).
func (s *Service) chooseSideloadVersion(id string) (version string, skipped []string, err error) {
	versionEntries, err := os.ReadDir(filepath.Join(s.pluginsDir, id))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil, nil
		}
		return "", nil, err
	}

	var candidates []string
	for _, v := range versionEntries {
		if !v.IsDir() {
			continue
		}
		manifestPath := filepath.Join(s.pluginsDir, id, v.Name(), manifestFileName)
		if _, statErr := os.Stat(manifestPath); statErr != nil {
			continue
		}
		candidates = append(candidates, v.Name())
	}
	if len(candidates) == 0 {
		return "", nil, nil
	}
	sort.Strings(candidates)
	chosen := candidates[len(candidates)-1]
	return chosen, candidates[:len(candidates)-1], nil
}

// manifestFileName mirrors pkgtar's own constant (kept local — pkgtar does
// not export it, and duplicating a single string literal here avoids
// exporting it just for this one read-only check).
const manifestFileName = "manifest.yaml"

// registerSideload parses+validates <pluginsDir>/<id>/<version>/manifest.yaml,
// requires it to be runtime-managed with a matching id, and persists a
// fresh StatusDisabled record for it.
func (s *Service) registerSideload(id, version string) error {
	versionDir := filepath.Join(s.pluginsDir, id, version)
	data, err := os.ReadFile(filepath.Join(versionDir, manifestFileName))
	if err != nil {
		return fmt.Errorf("read %s: %w", manifestFileName, err)
	}
	m, err := manifest.Parse(data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", manifestFileName, err)
	}
	if err := m.Validate(); err != nil {
		return fmt.Errorf("invalid %s: %w", manifestFileName, err)
	}
	if !m.IsManaged() {
		return fmt.Errorf("%s is not runtime-managed (runtime.type must be \"binary\")", manifestFileName)
	}
	if m.ID != id {
		return fmt.Errorf("manifest id %q does not match directory id %q", m.ID, id)
	}

	rec := &store.Record{
		Manifest:    *m,
		Status:      StatusDisabled,
		InstallPath: versionDir,
		Signed:      false,
		InstalledAt: time.Now().UTC(),
	}
	if err := s.store.Save(rec); err != nil {
		return fmt.Errorf("persist sideloaded record: %w", err)
	}
	s.registry.Add(rec)
	return nil
}

// scanTarballs installs every *.tar.gz file sitting directly under
// pluginsDir via the normal verified install pipeline (Service.Install),
// deleting each on success. A package that fails pkgtar's verify/validate
// pipeline is left in place and reported via errs.
func (s *Service) scanTarballs(ctx context.Context) (installed []string, errs []SyncError) {
	entries, err := os.ReadDir(s.pluginsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, []SyncError{{Path: s.pluginsDir, Reason: err.Error()}}
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), tarballSuffix) {
			continue
		}
		path := filepath.Join(s.pluginsDir, entry.Name())
		id, err := s.installTarball(ctx, path)
		if err != nil {
			errs = append(errs, SyncError{Path: path, Reason: err.Error()})
			continue
		}
		installed = append(installed, id)
	}
	return installed, errs
}

// installTarball opens path and installs it via Service.Install. A
// non-nil rec (even alongside a non-nil activation-only err — see Install's
// doc comment) means the package itself was valid and extracted, so the
// tarball is deleted; a nil rec means pkgtar rejected the package, so the
// file is left in place for the operator to fix.
func (s *Service) installTarball(ctx context.Context, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open dropped package: %w", err)
	}
	defer func() { _ = f.Close() }()

	rec, installErr := s.Install(ctx, f)
	if rec == nil {
		return "", installErr
	}
	if err := os.Remove(path); err != nil {
		s.log.Warn("plugins: sync failed to remove installed tarball",
			zap.String("path", path), zap.Error(err))
	}
	return rec.ID, nil
}

// scanMissingInstalls transitions every registered record whose
// InstallPath no longer exists on disk to StatusError (stopping its
// process first, if running), and returns their ids.
func (s *Service) scanMissingInstalls() []string {
	var missing []string
	for _, rec := range s.registry.List() {
		if rec.InstallPath == "" {
			continue
		}
		if _, err := os.Stat(rec.InstallPath); err == nil {
			continue
		}
		if s.runtime != nil && s.runtime.Running(rec.ID) {
			s.runtime.Stop(rec.ID)
		}
		s.markMissing(rec.ID)
		missing = append(missing, rec.ID)
	}
	return missing
}

// markMissing transitions id to StatusError via the normal FSM transition,
// falling back to a direct (transition-check-bypassing) status write if the
// transition is not a legal single-hop edge — e.g. the record is already
// StatusError, which SetStatus rejects as a same-status "transition".
func (s *Service) markMissing(id string) {
	err := s.SetStatus(id, StatusError)
	if err == nil {
		s.notifyDeliverer()
		return
	}
	var invalidErr *ErrInvalidTransition
	if !errors.As(err, &invalidErr) {
		s.log.Warn("plugins: sync failed to mark missing install as error",
			zap.String("plugin_id", id), zap.Error(err))
		return
	}
	if err := s.setStatusUnchecked(id, StatusError); err != nil {
		s.log.Warn("plugins: sync failed to force missing install to error",
			zap.String("plugin_id", id), zap.Error(err))
	}
}

// setStatusUnchecked directly writes status onto id's record in both the
// registry and the store, bypassing SetStatus's FSM transition check. Only
// used by markMissing's fallback path above.
func (s *Service) setStatusUnchecked(id string, status Status) error {
	updated, ok := s.registry.SetStatus(id, status)
	if !ok {
		return store.ErrNotFound
	}
	return s.store.Save(updated)
}
