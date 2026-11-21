package managedruntime

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/common/npmresolution"
)

const maxNpxPackageManifestBytes = 64 << 10

// CacheCondition identifies independently verified incomplete npm metadata.
type CacheCondition string

const CacheConditionRootManifestMissing CacheCondition = "root_manifest_missing"

// CacheEvidence is safe evidence for one exact managed runtime package.
type CacheEvidence struct {
	PackageSpec string
	Condition   CacheCondition
}

// NpxExecutionTreeInventory contains bounded facts read from one derived npm
// execution tree. It never exposes filesystem paths to callers.
type NpxExecutionTreeInventory struct {
	PackageSpec            string
	PackageName            string
	Version                string
	PackageInstalled       bool
	PackageIdentityMatches bool
	RootManifestMissing    bool
	BinSelectionValid      bool
	BinName                string
	BinTarget              string
	BinTargetPresent       bool
}

type npmPackageManifest struct {
	Name    string          `json:"name"`
	Version string          `json:"version"`
	Bin     json.RawMessage `json:"bin"`
}

// NpxExecutionRoot returns the deterministic execution root for an exact
// managed package specification.
func NpxExecutionRoot(cacheRoot, packageSpec string) (string, error) {
	if err := ValidateExactPackageSpec(packageSpec); err != nil {
		return "", err
	}
	root, err := validateNpmCacheRoot(cacheRoot)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "_npx", NpxExecutionCacheKey(packageSpec)), nil
}

// InspectNpxExecutionTree reads the installed top-level package from the
// exact npm execution key using no-follow directory and file handles.
func InspectNpxExecutionTree(cacheRoot, packageSpec string) (NpxExecutionTreeInventory, error) {
	var inventory NpxExecutionTreeInventory
	if err := ValidateExactPackageSpec(packageSpec); err != nil {
		return inventory, err
	}
	cacheRoot, err := validateNpmCacheRoot(cacheRoot)
	if err != nil {
		return inventory, err
	}
	// The final @ separates package name from the exact version, including for
	// scoped packages whose first character is also @.
	separator := strings.LastIndexByte(packageSpec, '@')
	packageName, version := packageSpec[:separator], packageSpec[separator+1:]
	inventory.PackageSpec = packageSpec
	inventory.PackageName = packageName
	inventory.Version = version

	cache, err := openNpxInventoryDirectoryPath(cacheRoot)
	if errors.Is(err, os.ErrNotExist) {
		return inventory, nil
	}
	if err != nil {
		return inventory, fmt.Errorf("open npm cache for inspection: %w", err)
	}
	defer func() { _ = cache.Close() }()
	npxRoot, err := openNpxInventoryDirectory(cache, "_npx")
	if errors.Is(err, os.ErrNotExist) {
		return inventory, nil
	}
	if err != nil {
		return inventory, fmt.Errorf("open npm execution cache for inspection: %w", err)
	}
	defer func() { _ = npxRoot.Close() }()
	executionRoot, err := openNpxInventoryDirectory(npxRoot, NpxExecutionCacheKey(packageSpec))
	if errors.Is(err, os.ErrNotExist) {
		return inventory, nil
	}
	if err != nil {
		return inventory, fmt.Errorf("open npm execution tree for inspection: %w", err)
	}
	defer func() { _ = executionRoot.Close() }()

	if err := inspectRootManifest(executionRoot, &inventory); err != nil {
		return inventory, err
	}
	return inventory, inspectInstalledNpxPackage(executionRoot, &inventory)
}

func inspectInstalledNpxPackage(executionRoot *os.File, inventory *NpxExecutionTreeInventory) error {
	packageDir, parents, err := openNpxInventoryDirectoryChain(executionRoot, append([]string{"node_modules"}, strings.Split(inventory.PackageName, "/")...))
	if packageDir != nil && packageDir != executionRoot {
		defer func() { _ = packageDir.Close() }()
	}
	for _, parent := range parents {
		defer func() { _ = parent.Close() }()
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect installed npm package: %w", err)
	}
	manifest, err := readNpxPackageManifest(packageDir)
	if err != nil {
		return fmt.Errorf("read installed npm package manifest: %w", err)
	}
	inventory.PackageInstalled = true
	if manifest.Name != inventory.PackageName || manifest.Version != inventory.Version {
		return nil
	}
	inventory.PackageIdentityMatches = true
	binName, binTarget, ok := selectedNpmPackageBin(inventory.PackageName, manifest.Bin)
	if !ok {
		return nil
	}
	inventory.BinSelectionValid = true
	inventory.BinName = binName
	inventory.BinTarget = binTarget
	inventory.BinTargetPresent = npxInventoryFileExists(packageDir, binTarget)
	return nil
}

// VerifyNpxRootManifestEvidence confirms npm's exact root-manifest failure
// against an independently inspected execution tree.
func VerifyNpxRootManifestEvidence(cacheRoot, packageSpec, stderr string) (CacheEvidence, bool) {
	var evidence CacheEvidence
	if err := ValidateExactPackageSpec(packageSpec); err != nil {
		return evidence, false
	}
	inventory, err := InspectNpxExecutionTree(cacheRoot, packageSpec)
	if err != nil || !inventory.PackageIdentityMatches || !inventory.RootManifestMissing || !inventory.BinSelectionValid {
		return evidence, false
	}
	executionRoot, err := NpxExecutionRoot(cacheRoot, packageSpec)
	if err != nil || !npmresolution.MatchesMissingExecutionRootManifest(stderr, filepath.Join(executionRoot, "package.json")) {
		return evidence, false
	}
	return CacheEvidence{PackageSpec: packageSpec, Condition: CacheConditionRootManifestMissing}, true
}

func inspectRootManifest(root *os.File, inventory *NpxExecutionTreeInventory) error {
	manifest, err := openNpxInventoryFile(root, "package.json")
	if errors.Is(err, os.ErrNotExist) {
		inventory.RootManifestMissing = true
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect npm execution root manifest: %w", err)
	}
	defer func() { _ = manifest.Close() }()
	data, err := io.ReadAll(io.LimitReader(manifest, maxNpxPackageManifestBytes+1))
	if err != nil {
		return fmt.Errorf("read npm execution root manifest: %w", err)
	}
	if len(data) > maxNpxPackageManifestBytes || !json.Valid(data) {
		return errors.New("npm execution root manifest is unreadable")
	}
	return nil
}

func readNpxPackageManifest(packageDir *os.File) (npmPackageManifest, error) {
	var manifest npmPackageManifest
	file, err := openNpxInventoryFile(packageDir, "package.json")
	if err != nil {
		return manifest, err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxNpxPackageManifestBytes+1))
	if err != nil {
		return manifest, err
	}
	if len(data) > maxNpxPackageManifestBytes {
		return manifest, errors.New("npm package manifest exceeds inspection limit")
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return manifest, err
	}
	return manifest, nil
}

func selectedNpmPackageBin(packageName string, raw json.RawMessage) (string, string, bool) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", "", false
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		single = strings.TrimPrefix(single, "./")
		return npmPackageBase(packageName), single, validPackageBinTarget(single)
	}
	var bins map[string]string
	if err := json.Unmarshal(raw, &bins); err != nil || len(bins) == 0 {
		return "", "", false
	}
	name := ""
	if len(bins) == 1 {
		for candidate := range bins {
			name = candidate
		}
	} else {
		name = npmPackageBase(packageName)
	}
	target, ok := bins[name]
	target = strings.TrimPrefix(target, "./")
	return name, target, ok && validNpmBinName(name) && validPackageBinTarget(target)
}

func npmPackageBase(packageName string) string {
	return packageName[strings.LastIndexByte(packageName, '/')+1:]
}

func validNpmBinName(name string) bool {
	return name != "" && filepath.Base(name) == name && !strings.ContainsAny(name, "\\/\x00")
}

func validPackageBinTarget(target string) bool {
	if target == "" || strings.ContainsAny(target, "\\\x00:") || strings.HasPrefix(target, "/") {
		return false
	}
	for _, component := range strings.Split(target, "/") {
		if component == "" || component == "." || component == ".." {
			return false
		}
	}
	return true
}

func npxInventoryFileExists(packageDir *os.File, target string) bool {
	components := strings.Split(target, "/")
	parent, parents, err := openNpxInventoryDirectoryChain(packageDir, components[:len(components)-1])
	for _, opened := range parents {
		defer func() { _ = opened.Close() }()
	}
	if err != nil {
		return false
	}
	defer func() { _ = parent.Close() }()
	file, err := openNpxInventoryFile(parent, components[len(components)-1])
	if err != nil {
		return false
	}
	_ = file.Close()
	return true
}

func openNpxInventoryDirectoryChain(root *os.File, components []string) (*os.File, []*os.File, error) {
	current := root
	opened := make([]*os.File, 0, len(components))
	for _, component := range components {
		next, err := openNpxInventoryDirectory(current, component)
		if err != nil {
			for _, file := range opened {
				_ = file.Close()
			}
			return nil, nil, err
		}
		opened = append(opened, next)
		current = next
	}
	if len(opened) == 0 {
		return root, opened, nil
	}
	return current, opened[:len(opened)-1], nil
}

func validInventoryEntryName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, "/\\\x00")
}
