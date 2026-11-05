package copilotplugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/tailscale/hujson"
)

const (
	maxNativeStateBytes      = 2 * 1024 * 1024
	maxPluginManifestBytes   = 256 * 1024
	maxMarketplaceFileBytes  = 256 * 1024
	maxInstalledPluginRows   = 1024
	maxMarketplacePluginRows = 1024
)

var errInstallationEvidenceUnavailable = errors.New("copilot plugin installation evidence unavailable")

type installedPlugin struct {
	Name        string
	Marketplace string
	Version     string
	CachePath   string
}

type installationState struct {
	Installed []installedPlugin
}

type marketplacePlugin struct {
	Name    string
	Source  string
	Version string
}

func resolveInventory(rows []InventoryRow, launch LaunchContext) error {
	var (
		state       installationState
		stateLoaded bool
		stateErr    error
	)
	needsState := false
	for _, row := range rows {
		if row.Source == SourceInstalled {
			needsState = true
			break
		}
	}
	if needsState {
		configDir, _, err := configDirForLaunch(launch)
		if err != nil {
			stateErr = err
		} else {
			state, stateErr = loadInstallationState(configDir)
			stateLoaded = stateErr == nil
		}
	}
	for index := range rows {
		row := &rows[index]
		if row.Availability == AvailabilityUnavailable {
			continue
		}
		switch row.Source {
		case SourceInstalled:
			if stateErr != nil || !stateLoaded {
				markUnavailable(row, ReasonMetadataUnavailable)
				continue
			}
			root, reason := resolveInstalledRoot(state, *row)
			if reason != "" {
				markUnavailable(row, reason)
				continue
			}
			row.root = root
		case SourceLive:
			root, reason := resolveLiveRoot(*row)
			if reason != "" {
				markUnavailable(row, reason)
				continue
			}
			row.root = root
		default:
			markUnavailable(row, ReasonUnsupportedSource)
		}
	}
	return nil
}

func markUnavailable(row *InventoryRow, reason ReasonCode) {
	row.Availability = AvailabilityUnavailable
	row.Reason = reason
	row.root = ""
}

func resolveInstalledRoot(state installationState, row InventoryRow) (string, ReasonCode) {
	matches := make([]installedPlugin, 0, 1)
	versionMismatch := false
	for _, entry := range state.Installed {
		if entry.Name != row.Name || entry.Marketplace != row.Marketplace {
			continue
		}
		if entry.Version != "" && row.Version != "" && entry.Version != row.Version {
			versionMismatch = true
			continue
		}
		matches = append(matches, entry)
	}
	if len(matches) == 0 {
		if versionMismatch {
			return "", ReasonMetadataUnavailable
		}
		return "", ReasonMetadataUnavailable
	}
	if len(matches) > 1 {
		return "", ReasonMetadataUnavailable
	}
	if matches[0].CachePath == "" {
		return "", ReasonMissingRoot
	}
	return validatePluginDirectory(matches[0].CachePath, row)
}

func resolveLiveRoot(row InventoryRow) (string, ReasonCode) {
	if row.InstalledFrom == "" {
		return "", ReasonMetadataUnavailable
	}
	if strings.Contains(row.InstalledFrom, "://") {
		return "", ReasonUnsupportedSource
	}
	marketplaceRoot, err := validateMarketplaceRoot(row.InstalledFrom)
	if err != nil {
		return "", ReasonUnsafeRoot
	}
	plugins, err := loadMarketplacePlugins(marketplaceRoot)
	if err != nil {
		return "", ReasonMetadataUnavailable
	}
	entry, ok := plugins[row.Name]
	if !ok {
		return "", ReasonMetadataUnavailable
	}
	if entry.Version != "" && row.Version != "" && entry.Version != row.Version {
		return "", ReasonMetadataUnavailable
	}
	if entry.Source == "" || unsafeMarketplaceSource(entry.Source) {
		return "", ReasonUnsupportedSource
	}
	candidate := filepath.Join(marketplaceRoot, entry.Source)
	return validatePluginDirectory(candidate, row)
}

func unsafeMarketplaceSource(source string) bool {
	if filepath.IsAbs(source) || strings.Contains(source, "://") || strings.HasPrefix(source, `\\`) {
		return true
	}
	for _, segment := range strings.FieldsFunc(source, func(r rune) bool { return r == '/' || r == '\\' }) {
		if segment == ".." {
			return true
		}
	}
	return false
}

func validateMarketplaceRoot(root string) (string, error) {
	if !filepath.IsAbs(root) || strings.ContainsRune(root, '\x00') {
		return "", errInstallationEvidenceUnavailable
	}
	clean := filepath.Clean(root)
	if clean != root || isVolumeRoot(clean) || isNetworkOrDevicePath(clean) {
		return "", errInstallationEvidenceUnavailable
	}
	info, err := os.Stat(clean)
	if err != nil || !info.IsDir() {
		return "", errInstallationEvidenceUnavailable
	}
	return clean, nil
}

func validatePluginDirectory(root string, row InventoryRow) (string, ReasonCode) {
	if root == "" {
		return "", ReasonMissingRoot
	}
	if !filepath.IsAbs(root) || strings.ContainsRune(root, '\x00') {
		return "", ReasonUnsafeRoot
	}
	clean := filepath.Clean(root)
	if clean != root || isVolumeRoot(clean) || isNetworkOrDevicePath(clean) {
		return "", ReasonUnsafeRoot
	}
	info, err := os.Stat(clean)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ReasonMissingRoot
		}
		return "", ReasonUnsafeRoot
	}
	if !info.IsDir() {
		return "", ReasonUnsafeRoot
	}
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return "", ReasonUnsafeRoot
	}
	resolvedInfo, err := os.Stat(resolved)
	if err != nil || !resolvedInfo.IsDir() || isVolumeRoot(resolved) {
		return "", ReasonUnsafeRoot
	}
	if reason := validatePluginManifest(resolved, row); reason != "" {
		return "", reason
	}
	return resolved, ""
}

func validatePluginManifest(root string, row InventoryRow) ReasonCode {
	manifestPath, found := firstExistingManifest(root)
	if !found {
		return ReasonManifestMismatch
	}
	info, err := os.Lstat(manifestPath)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return ReasonManifestMismatch
	}
	data, err := readBoundedFile(manifestPath, maxPluginManifestBytes)
	if err != nil {
		return ReasonManifestMismatch
	}
	standardized, err := hujson.Standardize(data)
	if err != nil || rejectDuplicateJSONKeys(standardized) != nil {
		return ReasonManifestMismatch
	}
	var manifest struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(standardized, &manifest); err != nil {
		return ReasonManifestMismatch
	}
	if strings.TrimSpace(manifest.Name) != row.Name {
		return ReasonManifestMismatch
	}
	if manifest.Version != "" && row.Version != "" && manifest.Version != row.Version {
		return ReasonManifestMismatch
	}
	return ""
}

func firstExistingManifest(root string) (string, bool) {
	for _, relative := range []string{
		"plugin.json",
		filepath.Join(".plugin", "plugin.json"),
		filepath.Join(".github", "plugin", "plugin.json"),
		filepath.Join(".claude-plugin", "plugin.json"),
	} {
		candidate := filepath.Join(root, relative)
		info, err := os.Lstat(candidate)
		if err == nil && !info.IsDir() {
			return candidate, true
		}
	}
	return "", false
}

func loadMarketplacePlugins(marketplaceRoot string) (map[string]marketplacePlugin, error) {
	manifestPath, found := firstExistingMarketplaceManifest(marketplaceRoot)
	if !found {
		return nil, errInstallationEvidenceUnavailable
	}
	info, statErr := os.Lstat(manifestPath)
	if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errInstallationEvidenceUnavailable
	}
	data, err := readBoundedFile(manifestPath, maxMarketplaceFileBytes)
	if err != nil {
		return nil, err
	}
	standardized, err := hujson.Standardize(data)
	if err != nil || rejectDuplicateJSONKeys(standardized) != nil {
		return nil, errInstallationEvidenceUnavailable
	}
	var manifest struct {
		Plugins []struct {
			Name    string          `json:"name"`
			Source  json.RawMessage `json:"source"`
			Version string          `json:"version"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(standardized, &manifest); err != nil {
		return nil, errInstallationEvidenceUnavailable
	}
	if len(manifest.Plugins) > maxMarketplacePluginRows {
		return nil, errInstallationEvidenceUnavailable
	}
	result := make(map[string]marketplacePlugin, len(manifest.Plugins))
	for _, entry := range manifest.Plugins {
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			return nil, errInstallationEvidenceUnavailable
		}
		source := ""
		if len(entry.Source) > 0 {
			if err := json.Unmarshal(entry.Source, &source); err != nil {
				// A github/url object source has no verified directory mapping.
				continue
			}
		}
		if _, duplicate := result[name]; duplicate {
			return nil, errInstallationEvidenceUnavailable
		}
		result[name] = marketplacePlugin{Name: name, Source: source, Version: strings.TrimSpace(entry.Version)}
	}
	return result, nil
}

func firstExistingMarketplaceManifest(root string) (string, bool) {
	for _, relative := range []string{
		"marketplace.json",
		filepath.Join(".plugin", "marketplace.json"),
		filepath.Join(".github", "plugin", "marketplace.json"),
		filepath.Join(".claude-plugin", "marketplace.json"),
	} {
		candidate := filepath.Join(root, relative)
		info, err := os.Lstat(candidate)
		if err == nil && !info.IsDir() {
			return candidate, true
		}
	}
	return "", false
}

func loadInstallationState(configDir string) (installationState, error) {
	configPath := filepath.Join(configDir, "config.json")
	data, err := readBoundedFile(configPath, maxNativeStateBytes)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return installationState{}, nil
		}
		return installationState{}, err
	}
	standardized, err := hujson.Standardize(data)
	if err != nil || rejectDuplicateJSONKeys(standardized) != nil {
		return installationState{}, errInstallationEvidenceUnavailable
	}
	var config struct {
		InstalledPlugins []struct {
			Name        string `json:"name"`
			Marketplace string `json:"marketplace"`
			Version     string `json:"version"`
			CachePath   string `json:"cache_path"`
		} `json:"installedPlugins"`
	}
	if err := json.Unmarshal(standardized, &config); err != nil {
		return installationState{}, errInstallationEvidenceUnavailable
	}
	if len(config.InstalledPlugins) > maxInstalledPluginRows {
		return installationState{}, errInstallationEvidenceUnavailable
	}
	state := installationState{Installed: make([]installedPlugin, 0, len(config.InstalledPlugins))}
	for _, entry := range config.InstalledPlugins {
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			return installationState{}, errInstallationEvidenceUnavailable
		}
		state.Installed = append(state.Installed, installedPlugin{
			Name:        name,
			Marketplace: strings.TrimSpace(entry.Marketplace),
			Version:     strings.TrimSpace(entry.Version),
			CachePath:   strings.TrimSpace(entry.CachePath),
		})
	}
	return state, nil
}

func readBoundedFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, errInstallationEvidenceUnavailable
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("native file exceeds %d bytes", limit)
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("native file exceeds %d bytes", limit)
	}
	return data, nil
}

func isVolumeRoot(path string) bool {
	cleaned := filepath.Clean(path)
	return filepath.Dir(cleaned) == cleaned
}

func isNetworkOrDevicePath(path string) bool {
	if strings.HasPrefix(path, `\\?\`) || strings.HasPrefix(path, `\\.\`) {
		return true
	}
	if runtime.GOOS == "windows" && strings.HasPrefix(path, `\\`) {
		return true
	}
	return false
}
