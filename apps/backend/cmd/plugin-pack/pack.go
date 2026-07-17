// Command plugin-pack packages a plugin source directory into the kandev
// plugin tar.gz format described in docs/plans/plugins/GRPC-CONTRACT.md §6:
// manifest.yaml plus any server/ui/assets files, with a generated
// checksums.txt covering everything else. It is a thin CLI wrapper around
// pkgtar/pkgtartest.WritePackage, reused by both apps/backend's
// `make e2e-plugin-package` target and (via `go run`, replace directive)
// external plugin repos such as kandev-plugin-hello.
//
// Usage:
//
//	plugin-pack -dir <plugin-package-dir> -out <file.tar.gz> [-platform-only]
package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/pkgtar/pkgtartest"
)

const (
	manifestFileName     = "manifest.yaml"
	checksumsFileName    = "checksums.txt"
	checksumsSigFileName = "checksums.txt.sig"
	serverDirPrefix      = "server/"
)

// PackOptions configures Pack.
type PackOptions struct {
	// PlatformOnly restricts included server/ executables to a single
	// platform (GOOS/GOARCH below), producing a smaller package — useful
	// for test fixtures that only ever run on the host that built them.
	PlatformOnly bool
	// GOOS/GOARCH override the platform used for PlatformOnly filtering.
	// Empty means runtime.GOOS/runtime.GOARCH (the normal case; overridable
	// for tests).
	GOOS   string
	GOARCH string
}

// Pack reads every file under dir (which must contain manifest.yaml) and
// writes a kandev plugin tar.gz package to w, preserving relative paths and
// generating checksums.txt. When opts.PlatformOnly is set, server/
// executables for platforms other than the resolved GOOS/GOARCH are
// excluded.
func Pack(dir string, w io.Writer, opts PackOptions) error {
	files, err := collectPackageFiles(dir, opts)
	if err != nil {
		return err
	}
	if _, ok := files[manifestFileName]; !ok {
		return fmt.Errorf("plugin-pack: %s not found under %s", manifestFileName, dir)
	}
	if err := pkgtartest.WritePackage(w, files); err != nil {
		return fmt.Errorf("plugin-pack: writing package: %w", err)
	}
	return nil
}

// collectPackageFiles walks dir and reads every regular file into a
// package-relative-path -> contents map, optionally filtering out
// non-host-platform server/ executables (see hostExecutablePath).
func collectPackageFiles(dir string, opts PackOptions) (map[string][]byte, error) {
	var hostExecPath string
	if opts.PlatformOnly {
		execPath, err := hostExecutablePath(dir, opts)
		if err != nil {
			return nil, err
		}
		hostExecPath = execPath
	}

	files := make(map[string][]byte)
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("plugin-pack: relative path for %s: %w", path, err)
		}
		rel = filepath.ToSlash(rel)
		if rel == checksumsFileName || rel == checksumsSigFileName {
			return fmt.Errorf("plugin-pack: %s must not be pre-supplied in %s; it is generated", rel, dir)
		}
		if opts.PlatformOnly && isServerExecutable(rel) && rel != hostExecPath {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("plugin-pack: reading %s: %w", rel, err)
		}
		files[rel] = data
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return files, nil
}

// hostExecutablePath parses dir/manifest.yaml and resolves the
// runtime.executables entry for opts.GOOS/opts.GOARCH (defaulting to
// runtime.GOOS/runtime.GOARCH).
func hostExecutablePath(dir string, opts PackOptions) (string, error) {
	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := opts.GOARCH
	if goarch == "" {
		goarch = runtime.GOARCH
	}

	data, err := os.ReadFile(filepath.Join(dir, manifestFileName))
	if err != nil {
		return "", fmt.Errorf("plugin-pack: reading %s for -platform-only: %w", manifestFileName, err)
	}
	m, err := manifest.Parse(data)
	if err != nil {
		return "", fmt.Errorf("plugin-pack: parsing %s: %w", manifestFileName, err)
	}
	execPath, ok := m.ExecutableFor(goos, goarch)
	if !ok {
		return "", fmt.Errorf("plugin-pack: manifest declares no runtime.executables entry for %s-%s", goos, goarch)
	}
	return execPath, nil
}

// isServerExecutable reports whether rel is a package-relative path under
// server/ (the directory pkgtartest/pkgtar treat as holding runtime
// executables).
func isServerExecutable(rel string) bool {
	return strings.HasPrefix(rel, serverDirPrefix)
}

// packToFile packs dir into a fresh file at out (creating parent
// directories as needed), overwriting any existing file.
func packToFile(dir, out string, platformOnly bool) error {
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return fmt.Errorf("plugin-pack: creating output dir: %w", err)
	}
	f, err := os.Create(out)
	if err != nil {
		return fmt.Errorf("plugin-pack: creating %s: %w", out, err)
	}
	defer func() { _ = f.Close() }()

	if err := Pack(dir, f, PackOptions{PlatformOnly: platformOnly}); err != nil {
		return err
	}
	return f.Close()
}
