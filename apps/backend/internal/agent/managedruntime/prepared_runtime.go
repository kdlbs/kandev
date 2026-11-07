package managedruntime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

const (
	preparedRuntimeOwnerMarker = ".kandev-managed-runtime-owner"
	preparedRuntimeWindowsOS   = "windows"
)

// ManagedRuntimeCommand is an owned process request for npm preparation.
type ManagedRuntimeCommand struct {
	Path string
	Args []string
	Dir  string
	Env  []string
	// RetainWhile prevents removal while the runner still owns consumers.
	RetainWhile func(func() bool)
}

// ManagedRuntimeCommandRunner starts and reaps an owned process tree.
type ManagedRuntimeCommandRunner func(context.Context, ManagedRuntimeCommand) error

// PreparedRuntime owns one private project and npm cache until its consumers stop.
type PreparedRuntime struct {
	root        string
	project     string
	cache       string
	packageSpec string
	ownerID     string
	mu          sync.Mutex
	closed      bool
	consumers   []func() bool
}

// PreparePrivateRuntime installs and validates one exact managed package in
// an isolated npm project and cache.
func PreparePrivateRuntime(
	ctx context.Context,
	packageSpec string,
	env []string,
	run ManagedRuntimeCommandRunner,
) (*PreparedRuntime, error) {
	if err := ValidateExactPackageSpec(packageSpec); err != nil {
		return nil, err
	}
	if run == nil {
		return nil, errors.New("managed runtime command runner is required")
	}
	tempParent, err := canonicalManagedRuntimeTempParent()
	if err != nil {
		return nil, err
	}
	root, err := os.MkdirTemp(tempParent, "kandev-managed-runtime-")
	if err != nil {
		return nil, fmt.Errorf("create private managed runtime root: %w", err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		_ = os.RemoveAll(root)
		return nil, fmt.Errorf("secure private managed runtime root: %w", err)
	}
	ownerID, err := newPreparedRuntimeOwnerID()
	if err != nil {
		_ = os.RemoveAll(root)
		return nil, fmt.Errorf("create private managed runtime owner marker: %w", err)
	}
	prepared := &PreparedRuntime{
		root: root, project: filepath.Join(root, "project"), cache: filepath.Join(root, "cache"),
		packageSpec: packageSpec, ownerID: ownerID,
	}
	if err := prepared.createOwnedRoot(); err != nil {
		return prepared.cleanupAfterPreparationFailure(err)
	}
	if err := ctx.Err(); err != nil {
		return prepared.cleanupAfterPreparationFailure(err)
	}
	command := ManagedRuntimeCommand{
		Path: "npm",
		Args: []string{
			"--prefix", prepared.project,
			"--cache", prepared.cache,
			"exec", "--yes", "--prefer-online", "--package=" + packageSpec,
			"--", "node", "-e", "",
		},
		Dir:         prepared.project,
		Env:         append([]string(nil), env...),
		RetainWhile: prepared.retainWhile,
	}
	if err := run(ctx, command); err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		} else {
			err = errors.New("private npm package preparation failed")
		}
		return prepared.cleanupAfterPreparationFailure(err)
	}
	if err := ctx.Err(); err != nil {
		return prepared.cleanupAfterPreparationFailure(err)
	}
	if err := prepared.validate(); err != nil {
		return prepared.cleanupAfterPreparationFailure(err)
	}
	return prepared, nil
}

func (p *PreparedRuntime) retainWhile(consumerLive func() bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.consumers = append(p.consumers, consumerLive)
}

func (p *PreparedRuntime) createOwnedRoot() error {
	markerPath := filepath.Join(p.root, preparedRuntimeOwnerMarker)
	marker, err := os.OpenFile(markerPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		_ = os.RemoveAll(p.root)
		return fmt.Errorf("create private managed runtime owner marker: %w", err)
	}
	if _, err := io.WriteString(marker, p.ownerID); err != nil {
		_ = marker.Close()
		return fmt.Errorf("write private managed runtime owner marker: %w", err)
	}
	if err := marker.Close(); err != nil {
		return fmt.Errorf("close private managed runtime owner marker: %w", err)
	}
	if err := os.Mkdir(p.project, 0o700); err != nil {
		return fmt.Errorf("create private npm project: %w", err)
	}
	if err := os.Mkdir(p.cache, 0o700); err != nil {
		return fmt.Errorf("create private npm cache: %w", err)
	}
	return nil
}

func (p *PreparedRuntime) cleanupAfterPreparationFailure(cause error) (*PreparedRuntime, error) {
	if err := p.Close(); err != nil {
		return p, errors.Join(cause, fmt.Errorf("retain private managed runtime after cleanup failure: %w", err))
	}
	return nil, cause
}

func (p *PreparedRuntime) validate() error {
	inventory, err := InspectNpxExecutionTree(p.cache, p.packageSpec)
	if err != nil {
		return fmt.Errorf("inspect private npm execution tree: %w", err)
	}
	if !inventory.PackageIdentityMatches || inventory.RootManifestMissing || !inventory.BinSelectionValid || !inventory.BinTargetPresent {
		return errors.New("private npm execution tree does not contain the selected package and executable")
	}
	root, err := NpxExecutionRoot(p.cache, p.packageSpec)
	if err != nil {
		return err
	}
	if err := validatePreparedNpmBinLink(root, inventory); err != nil {
		return fmt.Errorf("validate private npm executable link: %w", err)
	}
	return nil
}

// ProjectPrefix returns the private npm project root used by npx.
func (p *PreparedRuntime) ProjectPrefix() string {
	if p == nil {
		return ""
	}
	return p.project
}

// CacheDirectory returns the private npm cache used by npx.
func (p *PreparedRuntime) CacheDirectory() string {
	if p == nil {
		return ""
	}
	return p.cache
}

// PackageSpec returns the exact package@version owned by this preparation.
func (p *PreparedRuntime) PackageSpec() string {
	if p == nil {
		return ""
	}
	return p.packageSpec
}

// RewriteNpxCommand redirects only the trusted managed command's project and
// cache arguments to this prepared runtime.
func (p *PreparedRuntime) RewriteNpxCommand(command []string) ([]string, error) {
	if p == nil || len(command) < 2 || command[0] != "npx" {
		return nil, errors.New("managed runtime command is not a trusted npx command")
	}
	if err := ValidateExactPackageSpec(p.packageSpec); err != nil {
		return nil, err
	}
	packageIndex, err := preparedPackageIndex(command, p.packageSpec)
	if err != nil {
		return nil, err
	}
	args := append([]string(nil), command...)
	prefixIndex, err := preparedNpmOptionIndex(args, packageIndex, "--prefix", p.project)
	if err != nil {
		return nil, err
	}
	cacheIndex, err := preparedNpmOptionIndex(args, packageIndex, "--cache", p.cache)
	if err != nil {
		return nil, err
	}
	if prefixIndex < 0 {
		args = append(args[:packageIndex], append([]string{"--prefix", p.project}, args[packageIndex:]...)...)
	} else {
		args[prefixIndex+1] = p.project
	}
	if cacheIndex >= 0 {
		args[cacheIndex+1] = p.cache
	} else {
		packageIndex = packageArgIndex(args, p.packageSpec)
		args = append(args[:packageIndex], append([]string{"--cache", p.cache}, args[packageIndex:]...)...)
	}
	return args, nil
}

func preparedPackageIndex(command []string, packageSpec string) (int, error) {
	index := -1
	for position, arg := range command[1:] {
		if arg != packageSpec {
			continue
		}
		if index >= 0 {
			return 0, errors.New("managed runtime command repeats its package spec")
		}
		index = position + 1
	}
	if index < 0 {
		return 0, errors.New("managed runtime command does not select the prepared package")
	}
	return index, nil
}

func preparedNpmOptionIndex(args []string, packageIndex int, option, ownedValue string) (int, error) {
	found := -1
	for index := 1; index < packageIndex; index++ {
		if args[index] != option {
			continue
		}
		if found >= 0 || index+1 >= packageIndex {
			return 0, errors.New("managed runtime command repeats or omits an npm option")
		}
		if args[index+1] != NPMProjectPrefix && args[index+1] != ownedValue {
			return 0, errors.New("managed runtime command has an untrusted npm option")
		}
		found = index
		index++
	}
	return found, nil
}

func packageArgIndex(args []string, packageSpec string) int {
	for index, arg := range args {
		if arg == packageSpec {
			return index
		}
	}
	return len(args)
}

// Close removes only this verified owner root. Callers retain the owner after
// a cleanup error and may retry Close after consumer processes have stopped.
func (p *PreparedRuntime) Close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	for _, live := range p.consumers {
		if live() {
			return errors.New("private managed runtime consumers are still owned")
		}
	}
	info, err := os.Lstat(p.root)
	if errors.Is(err, os.ErrNotExist) {
		p.closed = true
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect private managed runtime owner: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("private managed runtime owner root is not a real directory")
	}
	markerInfo, err := os.Lstat(filepath.Join(p.root, preparedRuntimeOwnerMarker))
	if err != nil || !markerInfo.Mode().IsRegular() || markerInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("private managed runtime owner marker is unavailable")
	}
	marker, err := os.ReadFile(filepath.Join(p.root, preparedRuntimeOwnerMarker))
	if err != nil || string(marker) != p.ownerID {
		return errors.New("private managed runtime owner marker does not match")
	}
	if err := os.RemoveAll(p.root); err != nil {
		return fmt.Errorf("remove private managed runtime owner: %w", err)
	}
	p.closed = true
	return nil
}

func newPreparedRuntimeOwnerID() (string, error) {
	var value [24]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func validatePreparedNpmBinLink(executionRoot string, inventory NpxExecutionTreeInventory) error {
	binDirectory := filepath.Join(executionRoot, "node_modules", ".bin")
	packageDirectory := filepath.Join(executionRoot, "node_modules", filepath.FromSlash(inventory.PackageName))
	executable := filepath.Join(packageDirectory, filepath.FromSlash(inventory.BinTarget))
	if runtime.GOOS == preparedRuntimeWindowsOS {
		return validateWindowsNpmBinShim(binDirectory, inventory.BinName, executable)
	}
	link := filepath.Join(binDirectory, inventory.BinName)
	info, err := os.Lstat(link)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return errors.New("npm bin entry is not a symlink")
	}
	linkTarget, err := filepath.EvalSymlinks(link)
	if err != nil {
		return err
	}
	packageTarget, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return err
	}
	if !pathWithin(executionRoot, linkTarget) || !sameFilesystemPath(linkTarget, packageTarget) {
		return errors.New("npm bin symlink target is outside the selected package")
	}
	return nil
}

func validateWindowsNpmBinShim(binDirectory, binName, executable string) error {
	shim := filepath.Join(binDirectory, binName+".cmd")
	info, err := os.Lstat(shim)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("npm Windows bin shim is not a regular file")
	}
	file, err := os.Open(shim)
	if err != nil {
		return err
	}
	contents, readErr := io.ReadAll(io.LimitReader(file, 64<<10))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return errors.Join(readErr, closeErr)
	}
	if len(contents) == 64<<10 {
		return errors.New("npm Windows bin shim exceeds inspection limit")
	}
	relative, err := filepath.Rel(binDirectory, executable)
	if err != nil {
		return err
	}
	actual := strings.ToLower(strings.ReplaceAll(string(contents), "\\", "/"))
	expected := strings.ToLower(filepath.ToSlash(relative))
	if expected == "" || !strings.Contains(actual, expected) {
		return errors.New("npm Windows bin shim does not target the selected package")
	}
	return nil
}

func pathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func sameFilesystemPath(left, right string) bool {
	left, right = filepath.Clean(left), filepath.Clean(right)
	if runtime.GOOS == preparedRuntimeWindowsOS {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func canonicalManagedRuntimeTempParent() (string, error) {
	parent, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		return "", fmt.Errorf("resolve managed runtime temporary parent: %w", err)
	}
	parent, err = filepath.Abs(parent)
	if err != nil {
		return "", fmt.Errorf("normalize managed runtime temporary parent: %w", err)
	}
	return parent, nil
}
