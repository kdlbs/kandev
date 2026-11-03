package antigravityruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/acpcompat"
)

// ManifestName is the completion marker inside a managed bundle.
const ManifestName = ".kandev-antigravity-bundle.json"

// Bundle is one complete local runtime.
type Bundle struct {
	ServerPath  string
	HarnessPath string
	Version     string
	Digest      string
	Managed     bool
}

// Manifest records the reviewed identity of a completed managed bundle.
type Manifest struct {
	Version     string   `json:"version"`
	OS          string   `json:"os"`
	Arch        string   `json:"arch"`
	Digest      string   `json:"sha256"`
	Server      string   `json:"server"`
	Harness     string   `json:"harness"`
	ArtifactURL string   `json:"artifact_url"`
	ExtraFiles  []string `json:"extra_files,omitempty"`
}

// ResolveOptions supplies deterministic inputs for filesystem discovery.
type ResolveOptions struct {
	GOOS     string
	GOARCH   string
	HomeDir  string
	Env      map[string]string
	LookPath func(string) (string, error)
}

// DefaultHomeDir resolves the Kandev home used for managed bundles.
func DefaultHomeDir() string {
	if raw := strings.TrimSpace(os.Getenv("KANDEV_HOME_DIR")); raw != "" {
		return raw
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".kandev")
}

// ManagedRoot returns the provider subdirectory under the Kandev user cache.
func ManagedRoot(homeDir string) string {
	return filepath.Join(homeDir, "cache", "antigravity-acp")
}

// ManagedDestination returns the immutable destination for an artifact.
func ManagedDestination(homeDir string, artifact Artifact) string {
	digest := artifact.SHA256
	if len(digest) > 12 {
		digest = digest[:12]
	}
	return filepath.Join(ManagedRoot(homeDir), artifact.Version,
		fmt.Sprintf("%s-%s-%s", artifact.OS, artifact.Arch, digest))
}

// Resolve selects the managed bundle when its destination exists, otherwise a
// complete manual PATH install. An invalid managed destination is an error.
func Resolve(ctx context.Context, opts ResolveOptions) (*Bundle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	goos, goarch := opts.GOOS, opts.GOARCH
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	homeDir := opts.HomeDir
	if homeDir == "" {
		homeDir = DefaultHomeDir()
	}
	artifact, artifactErr := ArtifactFor(goos, goarch)
	if artifactErr != nil {
		return nil, artifactErr
	}
	if homeDir != "" {
		dest := ManagedDestination(homeDir, artifact)
		if info, err := os.Stat(dest); err == nil {
			if !info.IsDir() {
				return nil, fmt.Errorf("%w: %s is not a directory", ErrInvalidManagedBundle, dest)
			}
			return validateManagedBundle(ctx, dest, artifact)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: inspect %s: %v", ErrInvalidManagedBundle, dest, err)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return resolveManual(ctx, goos, artifact, opts)
}

func resolveManual(ctx context.Context, goos string, artifact Artifact, opts ResolveOptions) (*Bundle, error) {
	serverName := "agy_acp_server.par"
	if goos == windowsOS {
		serverName = "agy_acp_server.exe"
	}
	lookPath := opts.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	serverPath, err := lookPath(serverName)
	if err != nil {
		return nil, fmt.Errorf("%w: %s not found on PATH", ErrUnavailable, serverName)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	serverPath, err = filepath.Abs(serverPath)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve server path: %v", ErrUnavailable, err)
	}
	if !isRegularExecutable(serverPath, goos) {
		return nil, fmt.Errorf("%w: %s is not an executable regular file", ErrUnavailable, serverPath)
	}
	harnessPath, err := resolveManualHarness(goos, serverPath, opts.Env)
	if err != nil {
		return nil, err
	}
	version := artifact.Version
	if version == "" {
		version = Version
	}
	return &Bundle{
		ServerPath:  serverPath,
		HarnessPath: harnessPath,
		Version:     version,
		Managed:     false,
	}, nil
}

func resolveManualHarness(goos, serverPath string, env map[string]string) (string, error) {
	override := ""
	if env != nil {
		override = strings.TrimSpace(env[acpcompat.AntigravityHarnessPathEnv])
	} else {
		override = strings.TrimSpace(os.Getenv(acpcompat.AntigravityHarnessPathEnv))
	}
	if override != "" {
		abs, err := filepath.Abs(override)
		if err != nil {
			return "", fmt.Errorf("%w: %s", ErrInvalidHarnessOverride, override)
		}
		if !isRegularExecutable(abs, goos) {
			return "", fmt.Errorf("%w: %s is not an executable regular file", ErrInvalidHarnessOverride, abs)
		}
		return abs, nil
	}
	names := []string{"localharness_external", "localharness"}
	if goos == windowsOS {
		names = []string{"localharness_external.exe", "localharness.exe"}
	}
	dir := filepath.Dir(serverPath)
	for _, name := range names {
		candidate := filepath.Join(dir, name)
		if isRegularExecutable(candidate, goos) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%w: no localharness_external beside %s", ErrUnavailable, serverPath)
}

func validateManagedBundle(ctx context.Context, dir string, artifact Artifact) (*Bundle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if err != nil {
		return nil, fmt.Errorf("%w: read manifest: %v", ErrInvalidManagedBundle, err)
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("%w: parse manifest: %v", ErrInvalidManagedBundle, err)
	}
	if err := validateManifestIdentity(manifest, artifact); err != nil {
		return nil, err
	}
	server := filepath.Join(dir, manifest.Server)
	harness := filepath.Join(dir, manifest.Harness)
	if !isRegularExecutable(server, artifact.OS) || !isRegularExecutable(harness, artifact.OS) {
		return nil, fmt.Errorf("%w: bundle files are missing or not executable", ErrInvalidManagedBundle)
	}
	return &Bundle{
		ServerPath:  server,
		HarnessPath: harness,
		Version:     manifest.Version,
		Digest:      manifest.Digest,
		Managed:     true,
	}, nil
}

func validateManifestIdentity(manifest Manifest, artifact Artifact) error {
	if manifest.Version != artifact.Version || manifest.Digest != artifact.SHA256 ||
		manifest.OS != artifact.OS || manifest.Arch != artifact.Arch {
		return fmt.Errorf("%w: manifest identity does not match the reviewed artifact", ErrInvalidManagedBundle)
	}
	if manifest.Server != artifact.ServerName || filepath.Base(manifest.Server) != manifest.Server {
		return fmt.Errorf("%w: manifest server name is not the reviewed entry", ErrInvalidManagedBundle)
	}
	if filepath.Base(manifest.Harness) != manifest.Harness || !slices.Contains(artifact.HarnessNames, manifest.Harness) {
		return fmt.Errorf("%w: manifest harness name is not the reviewed entry", ErrInvalidManagedBundle)
	}
	return nil
}

func isRegularExecutable(path, goos string) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	if goos == windowsOS {
		return true
	}
	return info.Mode().Perm()&0o111 != 0
}

// IsManagedServerPath reports whether path is the active, validated managed
// server executable.
func IsManagedServerPath(ctx context.Context, path string) bool {
	bundle, err := Resolve(ctx, ResolveOptions{})
	return err == nil && bundle.Managed && sameServerFile(path, bundle.ServerPath)
}

// IsValidatedServerPath reports whether path is the active managed bundle or a
// complete manual PATH install selected by the resolver.
func IsValidatedServerPath(ctx context.Context, path string) bool {
	bundle, err := Resolve(ctx, ResolveOptions{})
	return err == nil && sameServerFile(path, bundle.ServerPath)
}

func sameServerFile(path, want string) bool {
	gotInfo, err := os.Stat(path)
	if err != nil {
		return false
	}
	wantInfo, err := os.Stat(want)
	if err != nil {
		return false
	}
	return os.SameFile(gotInfo, wantInfo)
}
