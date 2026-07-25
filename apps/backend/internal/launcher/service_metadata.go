package launcher

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	nativeServiceMetadataVersion = 1
	nativeServiceManagerSystemd  = "systemd"
	nativeServiceManagerLaunchd  = "launchd"
	nativeServiceModeUser        = "user"
	nativeServiceModeSystem      = "system"
	nativeInstallKindHomebrew    = "homebrew"
	nativeInstallKindNPM         = "npm"
	nativeInstallKindNPX         = "npx"
	nativeInstallKindLocal       = "local"
)

type nativeServiceMetadata struct {
	Version         int    `json:"version"`
	Manager         string `json:"manager"`
	Mode            string `json:"mode"`
	Kind            string `json:"kind"`
	HomeDir         string `json:"home_dir"`
	LogDir          string `json:"log_dir"`
	ServicePath     string `json:"service_path"`
	LauncherPath    string `json:"launcher_path"`
	BundleDir       string `json:"bundle_dir,omitempty"`
	LauncherVersion string `json:"launcher_version,omitempty"`
	Port            int    `json:"port,omitempty"`
	SystemUser      string `json:"system_user,omitempty"`
	NoBootStart     bool   `json:"no_boot_start,omitempty"`
	InstalledAt     string `json:"installed_at"`
}

func nativeServiceMetadataPath(homeDir string) string {
	return filepath.Join(homeDir, "service", "install.json")
}

func nativeServiceMode(system bool) string {
	if system {
		return nativeServiceModeSystem
	}
	return nativeServiceModeUser
}

func nativeInstallKind(executable, bundleDir string) string {
	paths := filepath.ToSlash(executable + "\n" + bundleDir)
	switch {
	case strings.Contains(paths, "/Cellar/kandev/"):
		return nativeInstallKindHomebrew
	case strings.Contains(paths, "/.npm/_npx/") || strings.Contains(paths, "/_npx/"):
		return nativeInstallKindNPX
	case strings.Contains(paths, "/node_modules/@kdlbs/runtime-") ||
		strings.Contains(paths, "/node_modules/kandev/"):
		return nativeInstallKindNPM
	default:
		return nativeInstallKindLocal
	}
}

func buildNativeServiceMetadata(
	manager string,
	args serviceArgs,
	input nativeServiceUnitInput,
	servicePath string,
) nativeServiceMetadata {
	return nativeServiceMetadata{
		Version:         nativeServiceMetadataVersion,
		Manager:         manager,
		Mode:            nativeServiceMode(args.System),
		Kind:            nativeInstallKind(input.Executable, input.BundleDir),
		HomeDir:         input.HomeDir,
		LogDir:          input.LogDir,
		ServicePath:     servicePath,
		LauncherPath:    input.Executable,
		BundleDir:       input.BundleDir,
		LauncherVersion: input.Version,
		Port:            input.Port,
		SystemUser:      input.SystemUser,
		NoBootStart:     input.NoBootStart,
		InstalledAt:     time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func writeNativeServiceMetadata(metadata nativeServiceMetadata) error {
	prepared, err := prepareNativeServiceMetadata(metadata)
	if err != nil {
		return err
	}
	defer func() { _ = prepared.Close() }()
	if err := prepared.Commit(); err != nil {
		return err
	}
	return prepared.Close()
}

type preparedNativeServiceMetadata struct {
	data        []byte
	homeRoot    nativeMetadataRoot
	serviceRoot nativeMetadataRoot
	owner       nativeServiceMetadataOwner
}

func prepareNativeServiceMetadata(metadata nativeServiceMetadata) (*preparedNativeServiceMetadata, error) {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal service metadata: %w", err)
	}
	homeRoot, err := openNativeMetadataHome(metadata)
	if err != nil {
		return nil, err
	}
	owner, err := resolveNativeServiceMetadataOwner(metadata)
	if err != nil {
		_ = homeRoot.Close()
		return nil, err
	}
	serviceRoot, err := inspectNativeServiceMetadataRoot(homeRoot)
	if err != nil {
		_ = homeRoot.Close()
		return nil, err
	}
	return &preparedNativeServiceMetadata{
		data:        append(data, '\n'),
		homeRoot:    homeRoot,
		serviceRoot: serviceRoot,
		owner:       owner,
	}, nil
}

func (p *preparedNativeServiceMetadata) Commit() error {
	if p.serviceRoot == nil {
		serviceRoot, err := createNativeServiceMetadataRoot(p.homeRoot)
		if err != nil {
			return err
		}
		p.serviceRoot = serviceRoot
	}
	return atomicWriteNativeServiceMetadata(p.serviceRoot, p.data, p.owner)
}

func (p *preparedNativeServiceMetadata) EnsureHomeDirectory(name string, mode os.FileMode) error {
	return ensureNativeMetadataDirectory(p.homeRoot, name, mode, p.owner)
}

func (p *preparedNativeServiceMetadata) Close() error {
	var closeErrs []error
	if p.serviceRoot != nil {
		closeErrs = append(closeErrs, p.serviceRoot.Close())
		p.serviceRoot = nil
	}
	if p.homeRoot != nil {
		closeErrs = append(closeErrs, p.homeRoot.Close())
		p.homeRoot = nil
	}
	return errors.Join(closeErrs...)
}

type nativeServiceMetadataOwner struct {
	UID     int
	GID     int
	Enabled bool
}

func resolveNativeServiceMetadataOwner(metadata nativeServiceMetadata) (nativeServiceMetadataOwner, error) {
	if metadata.Mode != nativeServiceModeSystem {
		return nativeServiceMetadataOwner{}, nil
	}
	if metadata.SystemUser == "" {
		return nativeServiceMetadataOwner{}, fmt.Errorf("system service metadata missing system_user")
	}
	uid, gid, err := lookupNativeServiceOwner(metadata.SystemUser)
	if err != nil {
		return nativeServiceMetadataOwner{}, fmt.Errorf("resolve system service user %q: %w", metadata.SystemUser, err)
	}
	return nativeServiceMetadataOwner{UID: uid, GID: gid, Enabled: true}, nil
}

func inspectNativeServiceMetadataRoot(homeRoot nativeMetadataRoot) (nativeMetadataRoot, error) {
	mode, err := homeRoot.EntryMode("service")
	switch {
	case os.IsNotExist(err):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("inspect service metadata dir: %w", err)
	case mode&os.ModeSymlink != 0:
		return nil, fmt.Errorf("service metadata dir must not be a symlink")
	case !mode.IsDir():
		return nil, fmt.Errorf("service metadata path is not a directory")
	}
	root, err := homeRoot.OpenRoot("service")
	if err != nil {
		return nil, fmt.Errorf("open service metadata dir: %w", err)
	}
	if err := rejectNativeMetadataSymlink(root); err != nil {
		_ = root.Close()
		return nil, err
	}
	return root, nil
}

func createNativeServiceMetadataRoot(homeRoot nativeMetadataRoot) (nativeMetadataRoot, error) {
	if err := homeRoot.Mkdir("service", 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, fmt.Errorf("create service metadata dir: %w", err)
	}
	root, err := inspectNativeServiceMetadataRoot(homeRoot)
	if err != nil {
		return nil, err
	}
	if root == nil {
		return nil, fmt.Errorf("service metadata dir does not exist after creation")
	}
	return root, nil
}

func ensureNativeMetadataDirectory(
	parent nativeMetadataRoot,
	name string,
	mode os.FileMode,
	owner nativeServiceMetadataOwner,
) error {
	entryMode, err := parent.EntryMode(name)
	switch {
	case os.IsNotExist(err):
		if err := parent.Mkdir(name, mode); err != nil {
			return fmt.Errorf("create %s directory: %w", name, err)
		}
	case err != nil:
		return fmt.Errorf("inspect %s directory: %w", name, err)
	case entryMode&os.ModeSymlink != 0:
		return fmt.Errorf("%s directory must not be a symlink", name)
	case !entryMode.IsDir():
		return fmt.Errorf("%s path is not a directory", name)
	}
	root, err := parent.OpenRoot(name)
	if err != nil {
		return fmt.Errorf("open %s directory: %w", name, err)
	}
	defer func() { _ = root.Close() }()
	dir, err := root.OpenSelf()
	if err != nil {
		return fmt.Errorf("open %s directory for ownership: %w", name, err)
	}
	defer func() { _ = dir.Close() }()
	if err := secureNativeMetadataFile(dir, mode, owner); err != nil {
		return fmt.Errorf("secure %s directory: %w", name, err)
	}
	return nil
}

func atomicWriteNativeServiceMetadata(
	serviceRoot nativeMetadataRoot,
	data []byte,
	owner nativeServiceMetadataOwner,
) error {
	if err := rejectNativeMetadataSymlink(serviceRoot); err != nil {
		return err
	}
	dir, err := serviceRoot.OpenSelf()
	if err != nil {
		return fmt.Errorf("open service metadata dir for ownership: %w", err)
	}
	if err := secureNativeMetadataFile(dir, 0o700, owner); err != nil {
		_ = dir.Close()
		return fmt.Errorf("secure service metadata dir: %w", err)
	}
	if err := dir.Close(); err != nil {
		return fmt.Errorf("close service metadata dir: %w", err)
	}

	tempName, err := nativeMetadataTempName()
	if err != nil {
		return err
	}
	file, err := serviceRoot.CreateExclusive(tempName, 0o600)
	if err != nil {
		return fmt.Errorf("create temporary service metadata: %w", err)
	}
	renamed := false
	defer func() {
		_ = file.Close()
		if !renamed {
			_ = serviceRoot.Remove(tempName)
		}
	}()
	if err := writeAndSecureNativeMetadata(file, data, owner); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temporary service metadata: %w", err)
	}
	if err := serviceRoot.Rename(tempName, "install.json"); err != nil {
		return fmt.Errorf("replace service metadata: %w", err)
	}
	renamed = true
	return nil
}

func rejectNativeMetadataSymlink(serviceRoot nativeMetadataRoot) error {
	mode, err := serviceRoot.EntryMode("install.json")
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect service metadata: %w", err)
	}
	if mode&os.ModeSymlink != 0 {
		return fmt.Errorf("service metadata file must not be a symlink")
	}
	if !mode.IsRegular() {
		return fmt.Errorf("service metadata path is not a regular file")
	}
	return nil
}

func nativeMetadataTempName() (string, error) {
	var suffix [12]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", fmt.Errorf("generate service metadata temp name: %w", err)
	}
	return ".install.json.tmp-" + hex.EncodeToString(suffix[:]), nil
}

func writeAndSecureNativeMetadata(
	file *os.File,
	data []byte,
	owner nativeServiceMetadataOwner,
) error {
	written, err := file.Write(data)
	if err != nil {
		return fmt.Errorf("write temporary service metadata: %w", err)
	}
	if written != len(data) {
		return fmt.Errorf("write temporary service metadata: %w", io.ErrShortWrite)
	}
	if err := secureNativeMetadataFile(file, 0o600, owner); err != nil {
		return fmt.Errorf("secure temporary service metadata: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync temporary service metadata: %w", err)
	}
	return nil
}

func secureNativeMetadataFile(
	file *os.File,
	mode os.FileMode,
	owner nativeServiceMetadataOwner,
) error {
	if err := file.Chmod(mode); err != nil {
		return err
	}
	if owner.Enabled {
		if err := chownNativeServiceMetadata(file, owner.UID, owner.GID); err != nil {
			return err
		}
	}
	return nil
}
