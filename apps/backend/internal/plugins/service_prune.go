package plugins

import (
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/plugins/manifest"
)

// pluginDataDirName is the plugin-owned writable directory the runtime
// manager creates beside a plugin's version directories
// (<pluginsDir>/<id>/data — see internal/plugins/runtime's pluginCommand). It
// is durable state that survives every upgrade and is removed only by
// Uninstall, so the prune below excludes it by name as well as by the
// manifest check every retained candidate has to pass.
const pluginDataDirName = "data"

// pruneSupersededVersions removes every superseded version directory under
// <pluginsDir>/<id>/, keeping activeVersion plus one rollback target.
// Installing never used to remove the version it replaced, so a plugin's
// directory grew by a full copy (~20MB, ~95MB for a package shipping all five
// platform binaries) on every install — and the auto-update poller installs
// with no user action at all, which is how a single plugin reached 46
// superseded copies and one machine reached 3.74GB of them.
//
// Retention is deliberately fixed at "current + 1 previous" rather than
// configurable: the second directory exists so a failed upgrade still has a
// working process to fall back on (see rollbackFailedInstall), which is a
// safety invariant rather than an operator preference, and any larger number
// re-creates the unbounded growth this exists to stop.
//
// Nothing is deleted unless id's process is confirmed running, which is the
// precondition that makes this safe: pruning before the new version starts
// would delete the very directory a failed upgrade needs. The check lives here
// rather than at each call site so a future caller cannot forget it — note
// that activate returns nil without starting anything when no runtime is
// wired, so "activate succeeded" alone is not the same guarantee.
//
// Every failure is logged and swallowed: a plugin that installed and started
// correctly must never be reported as failed because cleanup could not delete
// a directory. Callers hold id's lifecycle lock, so no concurrent
// install/enable/uninstall for the same id can race the scan.
//
// rollbackVersion names the version the caller knows was previously in use;
// when it is empty (or no longer on disk) the semver-greatest remaining
// version is retained instead.
func (s *Service) pruneSupersededVersions(id, activeVersion, rollbackVersion string) {
	if s.pluginsDir == "" || id == "" || activeVersion == "" {
		return
	}
	if s.runtime == nil || !s.runtime.Running(id) {
		return
	}

	// Every read and delete below goes through one os.Root handle opened on
	// the plugin directory, so a path validated by isInstalledVersionDir is
	// removed within the same confined tree: a symlink or rename planted
	// mid-prune cannot redirect the removal outside <pluginsDir>/<id>/.
	root, err := os.OpenRoot(filepath.Join(s.pluginsDir, id))
	if err != nil {
		if !os.IsNotExist(err) {
			s.log.Warn("plugins: could not open plugin directory for pruning",
				zap.String("plugin_id", id), zap.Error(err))
		}
		return
	}
	defer func() { _ = root.Close() }()

	versions, err := installedVersionDirs(root, id)
	if err != nil {
		s.log.Warn("plugins: could not scan installed versions for pruning",
			zap.String("plugin_id", id), zap.Error(err))
		return
	}

	keep := retainedVersions(versions, activeVersion, rollbackVersion)
	for _, version := range versions {
		if keep[version] {
			continue
		}
		// A concurrent Install extracts before it can take id's lifecycle
		// lock, so a version directory that exists here may belong to an
		// install still waiting on that lock. Deleting it would strand that
		// caller with an InstallPath pointing at nothing.
		if s.extractionInFlight(filepath.Join(s.pluginsDir, id, version)) {
			continue
		}
		if err := root.RemoveAll(version); err != nil {
			s.log.Warn("plugins: could not remove superseded plugin version",
				zap.String("plugin_id", id), zap.String("version", version), zap.Error(err))
			continue
		}
		s.log.Info("plugins: removed superseded plugin version",
			zap.String("plugin_id", id), zap.String("version", version))
	}
}

// pruneUnderLifecycleLock is pruneSupersededVersions for a caller that does
// not already hold id's lifecycle lock (the boot activation loop). Install
// holds it across activate and the prune that follows, and the prune relies on
// that exclusion, so the boot path takes the same lock rather than depending
// on nothing else being able to run yet.
func (s *Service) pruneUnderLifecycleLock(id, activeVersion, rollbackVersion string) {
	lock := s.lifecycleLocks.lockFor(id)
	lock.Lock()
	defer lock.Unlock()
	s.pruneSupersededVersions(id, activeVersion, rollbackVersion)
}

// retainedVersions returns the set of version directory names to keep:
// active, plus one rollback target — rollback when the caller named a version
// that is still on disk, otherwise the semver-greatest of the rest.
func retainedVersions(versions []string, active, rollback string) map[string]bool {
	keep := map[string]bool{active: true}
	fallback := ""
	for _, version := range versions {
		if version == active {
			continue
		}
		if version == rollback {
			keep[version] = true
			return keep
		}
		if fallback == "" || manifest.CompareVersions(version, fallback) > 0 {
			fallback = version
		}
	}
	if fallback != "" {
		keep[fallback] = true
	}
	return keep
}

// installedVersionDirs lists the entries directly under root (a handle on
// <pluginsDir>/<id>/) that are installed versions of id. The bar is
// deliberately high, because everything it returns is a deletion candidate:
// the entry must be a real directory (a symlink reports as a link here and is
// skipped), must not be the plugin's data directory or any dot-prefixed name
// (which covers pkgtar's in-flight ".tmp-*" staging directories and any other
// hidden entry), and must hold a manifest.yaml naming exactly this id and this
// directory as its version — which is what pkgtar.Install writes and what
// nothing else on that path produces. The plugin's store record ("<id>.yml")
// and config ("<id>.config.yml") live one level up, beside the plugin
// directory rather than inside it, so the root handle cannot reach them.
func installedVersionDirs(root *os.Root, id string) ([]string, error) {
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer func() { _ = dir.Close() }()

	entries, err := dir.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	var versions []string
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || name == pluginDataDirName || strings.HasPrefix(name, ".") {
			continue
		}
		if !isInstalledVersionDir(root, id, name) {
			continue
		}
		versions = append(versions, name)
	}
	return versions, nil
}

// isInstalledVersionDir reports whether <version>/manifest.yaml under root
// parses and declares exactly this id and version.
func isInstalledVersionDir(root *os.Root, id, version string) bool {
	data, err := root.ReadFile(filepath.Join(version, manifestFileName))
	if err != nil {
		return false
	}
	m, err := manifest.Parse(data)
	if err != nil {
		return false
	}
	return m.ID == id && m.Version == version
}
