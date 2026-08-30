package plugins

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/pkgtar"
	"github.com/kandev/kandev/internal/plugins/store"
)

// maxDownloadSize caps the response body InstallFromURL will read, per the
// task's build instructions (100MB cap).
const maxDownloadSize = 100 << 20

// downloadTimeout bounds how long InstallFromURL waits for the whole
// download.
const downloadTimeout = 60 * time.Second

// Install verifies and extracts r (a tar.gz plugin package) via pkgtar into
// the plugins directory, persists a fresh store.Record (status
// "registered"), adds it to the in-memory registry, and attempts to spawn
// and activate it. A pkgtar error (e.g. pkgtar.ErrVersionExists) is
// returned unchanged so callers can map it to the right HTTP status. If the
// package is valid but the initial spawn fails, the record is still
// persisted (status "error") and returned alongside the spawn error, so an
// operator can fix the issue and retry via Enable.
//
// Installing a new version of a plugin id that is currently active/running
// stops the old process first (activate's own "already running" idempotency
// check would otherwise skip spawning entirely, leaving the live subprocess
// running the OLD version's binary even though the record/install_path now
// point at the new one). If persisting the fresh record then fails,
// rollbackFailedInstall removes only the just-extracted version directory
// (every other installed version, and the plugin's writable data directory,
// survive) and restarts the previous version's process, so a failed upgrade
// attempt never destroys a previously working install.
func (s *Service) Install(ctx context.Context, r io.Reader) (*store.Record, error) {
	result, err := s.extractPackage(r)
	if err != nil {
		return nil, err
	}
	defer s.releaseExtraction(result.InstallPath)
	if err := s.checkMinKandevVersion(result.Manifest.MinKandevVersion); err != nil {
		_ = os.RemoveAll(result.InstallPath)
		return nil, err
	}
	s.warnWebhookAccessIssues(*result.Manifest)
	s.agentToolInstallMu.Lock()
	catalogLocked := true
	defer func() {
		if catalogLocked {
			s.agentToolInstallMu.Unlock()
		}
	}()
	if err := s.validateAgentToolInstall(result.Manifest); err != nil {
		_ = os.RemoveAll(result.InstallPath)
		return nil, err
	}

	// The plugin id is only known once pkgtar.Install has parsed the
	// package's manifest, so the per-plugin lock is acquired here rather
	// than at the very top of the function — this still covers
	// InstallFromURL, which calls through to Install. It serializes the
	// rest of this method (the record/registry/activate mutation) against
	// any other Enable/Disable/Install/Uninstall/UpdateConfig call for the
	// same id.
	lock := s.lifecycleLocks.lockFor(result.Manifest.ID)
	lock.Lock()
	defer lock.Unlock()
	dispatchLock := s.dispatchLocks.lockFor(result.Manifest.ID)
	dispatchLock.Lock()
	defer dispatchLock.Unlock()

	oldRec, hadOldRec := s.registry.Get(result.Manifest.ID)
	if err := s.ensureOwnershipAvailable(result.Manifest); err != nil {
		// pkgtar.Install has already atomically extracted exactly this new
		// version before manifest-wide active-owner checks can run. Remove
		// only that fresh version on rejection; otherwise a failed ownership
		// check strands it and turns a later valid install into ErrVersionExists.
		_ = os.RemoveAll(result.InstallPath)
		return nil, err
	}
	wasRunning := s.runtime != nil && s.runtime.Running(result.Manifest.ID)
	// Replacing an active plugin is an unload/reload boundary. Its old
	// credential binding may no longer describe the successor, even when both
	// package versions declare the same provider, so revoke before stopping the
	// old runtime and exposing the new record.
	if hadOldRec && oldRec.Status == StatusActive {
		s.revokeGitCredentialProviderLeases(oldRec.RepositoryProviders)
	}
	if wasRunning {
		s.runtime.Stop(result.Manifest.ID)
	}

	rec := &store.Record{
		Manifest:    *result.Manifest,
		Status:      StatusRegistered,
		InstallPath: result.InstallPath,
		Signed:      result.Signed,
		InstalledAt: time.Now().UTC(),
	}
	// An in-place upgrade rebuilds the record from the new package's manifest,
	// so the operator's per-plugin auto-update override (an operator choice,
	// not a manifest fact) must be carried forward or an auto-update would
	// silently reset the very toggle that triggered it.
	if hadOldRec {
		rec.AutoUpdate = oldRec.AutoUpdate
	}
	if err := s.store.Save(rec); err != nil {
		s.rollbackFailedInstall(result.InstallPath, oldRec, hadOldRec && wasRunning)
		return nil, fmt.Errorf("plugins: persist installed record: %w", err)
	}
	s.registry.Add(rec)
	s.agentToolInstallMu.Unlock()
	catalogLocked = false

	activateErr := s.activate(rec)
	if activateErr == nil {
		// Only now can the new version be confirmed running, which is the
		// point at which the versions it superseded stop being rollback
		// targets. Pruning any earlier would delete what a failed upgrade
		// falls back to; pruneSupersededVersions re-checks that the process is
		// actually up before deleting anything.
		s.pruneSupersededVersions(rec.ID, rec.Version, previousVersion(oldRec, hadOldRec))
	}
	s.notifyDeliverer()
	s.notifyAgentToolCatalogChanged()

	installed, getErr := s.Get(rec.ID)
	if getErr != nil {
		return rec, activateErr
	}
	return installed, activateErr
}

// extractPackage runs pkgtar.Install and registers the extracted version
// directory as an in-flight install, both under extractingMu so the directory
// is never visible on disk without being marked. Install cannot take the
// per-plugin lifecycle lock any earlier than this — the id is only known once
// the package's manifest has been parsed — so without the mark, two
// overlapping installs of the same id would both extract, and whichever
// acquired the lock first would prune the other's fresh directory and leave it
// activating an InstallPath that no longer exists.
//
// The mutex serializes extraction across every plugin, not just one id. That
// is deliberate and cheap: an install is a rare operator- or poller-driven
// action, and the alternative (marking after pkgtar returns) leaves exactly
// the gap this exists to close.
func (s *Service) extractPackage(r io.Reader) (*pkgtar.InstallResult, error) {
	s.extractingMu.Lock()
	defer s.extractingMu.Unlock()

	result, err := pkgtar.Install(r, s.pluginsDir)
	if err != nil {
		return nil, err
	}
	if s.extractingPaths == nil {
		s.extractingPaths = make(map[string]int)
	}
	s.extractingPaths[result.InstallPath]++
	return result, nil
}

// releaseExtraction drops one in-flight mark for path, deferred by Install so
// it runs however that install ends (rejected package, failed persist, or a
// completed activation).
func (s *Service) releaseExtraction(path string) {
	s.extractingMu.Lock()
	defer s.extractingMu.Unlock()
	if s.extractingPaths[path] > 1 {
		s.extractingPaths[path]--
		return
	}
	delete(s.extractingPaths, path)
}

// extractionInFlight reports whether some install has extracted path and not
// yet finished with it.
func (s *Service) extractionInFlight(path string) bool {
	s.extractingMu.Lock()
	defer s.extractingMu.Unlock()
	return s.extractingPaths[path] > 0
}

// previousVersion returns the version an in-place upgrade replaced, or "" when
// this install had no existing record to replace (a first install, or one over
// a plugin whose record was lost) and therefore knows of no rollback target.
func previousVersion(oldRec *store.Record, hadOldRec bool) string {
	if !hadOldRec || oldRec == nil {
		return ""
	}
	return oldRec.Version
}

// DevKandevVersion is the version string an un-stamped local build carries
// (cmd/kandev's `Version` default, mirrored by internal/system/updates'
// devVersion). It sorts meaninglessly against real semver, and a developer
// running from source must still be able to install a package that declares
// a min_kandev_version, so it disables the check entirely.
const DevKandevVersion = "dev"

// checkMinKandevVersion rejects a package whose manifest declares a
// min_kandev_version newer than the currently running release build. Release
// tags may carry a leading `v`; development and git-describe build strings do
// not provide a trustworthy release boundary, so they deliberately skip this
// release-only compatibility gate. An invalid manifest minimum is rejected.
func (s *Service) checkMinKandevVersion(minVersion string) error {
	if minVersion == "" || s.kandevVersion == "" || s.kandevVersion == DevKandevVersion {
		return nil
	}
	runningVersion, runningRelease := manifest.NormalizeReleaseVersion(s.kandevVersion)
	if !runningRelease {
		return nil
	}
	minimumVersion, minimumRelease := manifest.NormalizeReleaseVersion(minVersion)
	if !minimumRelease {
		return fmt.Errorf("plugins: min_kandev_version %q is not a release version", minVersion)
	}
	if manifest.CompareVersions(runningVersion, minimumVersion) < 0 {
		return fmt.Errorf("plugins: requires kandev >= %s, running %s", minVersion, s.kandevVersion)
	}
	return nil
}

// rollbackFailedInstall cleans up after a store.Save failure partway
// through Install: it removes only freshInstallPath (the version directory
// pkgtar.Install just extracted), never the whole destRoot/<id>/ tree —
// other installed versions and the plugin's writable data directory
// (destRoot/<id>/data) must survive. If restartOld is true (an existing
// record was running and got stopped to make way for this install),
// oldRec's process is best-effort restarted so the failed upgrade attempt
// doesn't also take down the previously working version; a restart failure
// is logged, not returned, since Install is already returning the original
// Save error.
func (s *Service) rollbackFailedInstall(freshInstallPath string, oldRec *store.Record, restartOld bool) {
	if err := os.RemoveAll(freshInstallPath); err != nil {
		s.log.Warn("plugins: failed to remove extracted package after a persist failure",
			zap.String("install_path", freshInstallPath), zap.Error(err))
	}
	if !restartOld || s.runtime == nil || oldRec == nil {
		return
	}
	startCtx, cancel := context.WithTimeout(context.Background(), activateStartTimeout)
	defer cancel()
	if err := s.runtime.Start(startCtx, oldRec, s.hostForPlugin); err != nil {
		s.log.Warn("plugins: failed to restart previous version after a failed upgrade",
			zap.String("plugin_id", oldRec.ID), zap.Error(err))
	}
}

// InstallFromURL downloads url (capped at maxDownloadSize, bounded by
// downloadTimeout) and installs it via Install. The HTTP route is admin-only;
// validateInstallURL also rejects non-http(s) schemes and malformed targets
// before any request is built.
func (s *Service) InstallFromURL(ctx context.Context, url string) (*store.Record, error) {
	if err := validateInstallURL(url); err != nil {
		return nil, fmt.Errorf("plugins: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("plugins: build download request: %w", err)
	}
	// The only HTTP caller is guarded by RequireAdmin; this operator-level
	// feature intentionally fetches the validated package URL.
	// lgtm[go/request-forgery]
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("plugins: download package: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("plugins: download package: server responded %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDownloadSize+1))
	if err != nil {
		return nil, fmt.Errorf("plugins: read package: %w", err)
	}
	if int64(len(data)) > maxDownloadSize {
		return nil, fmt.Errorf("plugins: package exceeds max download size of %d bytes", maxDownloadSize)
	}

	return s.Install(ctx, bytes.NewReader(data))
}

// validateInstallURL is the sink-level guard InstallFromURL applies before
// building any outbound request: raw must parse as a URL with an http or
// https scheme and a non-empty host. It rejects file://, gopher://, and
// other schemes that would let an operator-supplied string reach something
// other than a plain HTTP(S) fetch. This narrows, but does not eliminate,
// the residual SSRF surface inherent to letting an operator point the
// installer at an arbitrary http(s) URL (including internal hosts).
func validateInstallURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid install URL: %w", err)
	}
	switch parsed.Scheme {
	case "http", "https":
	default:
		return fmt.Errorf("invalid install URL: unsupported scheme %q (must be http or https)", parsed.Scheme)
	}
	if parsed.Hostname() == "" {
		return errors.New("invalid install URL: missing host")
	}
	return nil
}

// Uninstall stops id's process (if running), purges its vault namespace and
// every plugin_user_state row, removes its extracted package tree from disk,
// deletes its record from both the store and the in-memory registry, and
// deletes every plugin_state row scoped to id (best-effort — a failure there
// is logged but does not fail the overall Uninstall, since the package/record
// are already gone by that point), then notifies the attached Deliverer.
// Clearing plugin_state, plugin_user_state, and the vault namespace matters so
// a plugin reinstalled under the same id (or an id later reused by a different
// plugin) never silently inherits stale state or secrets.
//
// Ordering is deliberate: the process is stopped FIRST so the plugin can no
// longer race the cleanup by writing a fresh secret (SetSecret) between the
// vault list and the deletes; then the vault namespace is purged and any
// failure aborts the uninstall — nothing destructive (package/record
// removal) has happened yet, so the operator simply retries (Stop, the vault
// deletes, and the user-state purge are all idempotent). A failed uninstall
// therefore leaves the plugin stopped-but-installed, resolved by a retry.
func (s *Service) Uninstall(ctx context.Context, id string) error {
	lock := s.lifecycleLocks.lockFor(id)
	lock.Lock()
	defer lock.Unlock()
	dispatchLock := s.dispatchLocks.lockFor(id)
	dispatchLock.Lock()
	defer dispatchLock.Unlock()

	rec, err := s.Get(id)
	if err != nil {
		return err
	}
	wasRunning := s.runtime != nil && s.runtime.Running(id)
	if s.runtime != nil {
		s.runtime.Stop(id)
	}
	s.revokeGitCredentialProviderLeases(rec.RepositoryProviders)
	if err := s.deletePluginSecrets(ctx, id); err != nil {
		s.reconcileAbortedUninstall(id, wasRunning)
		return fmt.Errorf("plugins: uninstall aborted, could not purge plugin secrets: %w", err)
	}
	if err := s.deletePluginUserState(ctx, id); err != nil {
		s.reconcileAbortedUninstall(id, wasRunning)
		return fmt.Errorf("plugins: uninstall aborted, could not purge plugin user state: %w", err)
	}
	if err := pkgtar.Remove(s.pluginsDir, id); err != nil {
		return fmt.Errorf("plugins: remove installed package: %w", err)
	}
	if err := s.store.Delete(id); err != nil {
		return err
	}
	s.registry.Remove(id)
	s.deletePluginState(id)
	s.notifyDeliverer()
	s.notifyAgentToolCatalogChanged()
	return nil
}

func (s *Service) reconcileAbortedUninstall(id string, wasRunning bool) {
	if !wasRunning {
		return
	}
	if setErr := s.SetStatus(id, StatusError); setErr != nil {
		s.log.Warn("plugins: could not mark plugin errored after an aborted uninstall",
			zap.String("plugin_id", id), zap.Error(setErr))
	}
	s.notifyDeliverer()
}

// deletePluginSecrets removes every vault entry in id's namespace
// ("plugin:<id>:..." — both SetSecret-owned and config-backed entries), so
// a reinstall under the same id never inherits stale secrets. Unlike
// deletePluginState it is NOT best-effort: it runs before any destructive
// uninstall step (after the process is stopped, so no concurrent writes can
// re-populate the namespace), and a failure aborts the uninstall while it
// can still be retried. Deletion is idempotent, so a partial failure that
// deleted some entries is safely resumed by a retry. A nil vault (no
// secrets possible) is a no-op.
func (s *Service) deletePluginSecrets(ctx context.Context, id string) error {
	if s.secrets == nil {
		return nil
	}
	ids, err := s.secrets.ListIDs(ctx)
	if err != nil {
		return fmt.Errorf("list vault ids: %w", err)
	}
	var errs []error
	for _, vaultID := range ids {
		if !hasPluginVaultPrefix(vaultID, id) {
			continue
		}
		if err := s.secrets.Delete(ctx, vaultID); err != nil {
			errs = append(errs, fmt.Errorf("delete %s: %w", vaultID, err))
		}
	}
	return errors.Join(errs...)
}

// deletePluginState best-effort removes every plugin_state row for id. A
// nil state store (e.g. a Service constructed without SetState in tests, or
// before backendapp finishes wiring) is a silent no-op.
func (s *Service) deletePluginState(id string) {
	if s.state == nil {
		return
	}
	if err := s.state.DeleteAll(context.Background(), id); err != nil {
		s.log.Warn("plugins: failed to delete plugin_state on uninstall", zap.String("plugin_id", id), zap.Error(err))
	}
}

// deletePluginUserState removes every plugin_user_state row for id, across
// every user (AC20). A nil cleanup store is a silent no-op for narrowly
// constructed tests where per-user storage could never have been written.
func (s *Service) deletePluginUserState(ctx context.Context, id string) error {
	if s.userStateCleanup == nil {
		return nil
	}
	return s.userStateCleanup.DeleteAllForPlugin(ctx, id)
}
