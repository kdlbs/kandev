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
// The caller must have confirmed that activeVersion's process started
// cleanly — pruning before that would delete the very directory a failed
// upgrade needs. Every failure here is logged and swallowed: a plugin that
// installed and started correctly must never be reported as failed because
// cleanup could not delete a directory.
//
// rollbackVersion names the version the caller knows was previously in use;
// when it is empty (or no longer on disk) the semver-greatest remaining
// version is retained instead.
func (s *Service) pruneSupersededVersions(id, activeVersion, rollbackVersion string) {
	if s.pluginsDir == "" || id == "" || activeVersion == "" {
		return
	}
	versions, err := s.installedVersionDirs(id)
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
		dir := filepath.Join(s.pluginsDir, id, version)
		if err := os.RemoveAll(dir); err != nil {
			s.log.Warn("plugins: could not remove superseded plugin version",
				zap.String("plugin_id", id), zap.String("path", dir), zap.Error(err))
			continue
		}
		s.log.Info("plugins: removed superseded plugin version",
			zap.String("plugin_id", id), zap.String("version", version))
	}
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

// installedVersionDirs lists the directories under <pluginsDir>/<id>/ that
// are installed versions of id. The bar is deliberately high, because
// everything it returns is a deletion candidate: the entry must be a real
// directory (a symlink is never followed, let alone removed), must not be the
// plugin's data directory or one of pkgtar's in-flight ".tmp-*" staging
// dirs, and must hold a manifest.yaml naming exactly this id and this
// directory as its version — which is what pkgtar.Install writes and what
// nothing else on that path produces. The plugin's store record
// ("<id>.yml") and config ("<id>.config.yml") live one level up, beside the
// plugin directory rather than inside it, so they are out of reach here.
func (s *Service) installedVersionDirs(id string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(s.pluginsDir, id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var versions []string
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || name == pluginDataDirName || strings.HasPrefix(name, ".") {
			continue
		}
		if !s.isInstalledVersionDir(id, name) {
			continue
		}
		versions = append(versions, name)
	}
	return versions, nil
}

// isInstalledVersionDir reports whether <pluginsDir>/<id>/<version> holds a
// manifest.yaml that parses and declares exactly this id and version.
func (s *Service) isInstalledVersionDir(id, version string) bool {
	data, err := os.ReadFile(filepath.Join(s.pluginsDir, id, version, manifestFileName))
	if err != nil {
		return false
	}
	m, err := manifest.Parse(data)
	if err != nil {
		return false
	}
	return m.ID == id && m.Version == version
}
