package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// appsDir is the apps/ workspace root relative to the working directory (apps/backend/).
const appsDir = ".."
const repoRootDir = "../.."

// goDockerImage is used to cross-compile CGO binaries on non-linux/amd64 hosts.
const goDockerImage = "golang:1.26-bookworm"

// buildLinuxBinaries compiles kandev, agentctl, and mock-agent for linux/amd64.
// Run this from apps/backend/. agentctl and mock-agent use CGO_ENABLED=0 and
// always build natively. kandev requires CGO (SQLite) and is always built inside
// a Docker container to target a known glibc version (Debian Bookworm = 2.36).
func buildLinuxBinaries(ctx context.Context, outDir string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	// agentctl and mock-agent don't need CGO — build natively with cross-env.
	for _, b := range []struct{ name, pkg string }{
		{"agentctl", "./cmd/agentctl"},
		{"mock-agent", "./cmd/mock-agent"},
	} {
		out := filepath.Join(outDir, b.name)
		fmt.Fprintf(os.Stderr, "  go build %s -> %s\n", b.pkg, out)
		cmd := exec.CommandContext(ctx, "go", "build", "-ldflags", "-s -w", "-o", out, b.pkg)
		cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("build %s: %w", b.name, err)
		}
	}

	// kandev requires CGO for SQLite. Always build inside Docker so the resulting
	// binary targets a known glibc version (Debian Bookworm = 2.36) regardless of
	// the host OS. Sprites VMs run a compatible glibc; building natively on the
	// CI runner (Ubuntu 24.04, glibc 2.39) would produce a binary that requires
	// symbols unavailable in the Sprites environment.
	kandevOut := filepath.Join(outDir, "kandev")
	fmt.Fprintf(os.Stderr, "  go build ./cmd/kandev (docker cross-compile) -> %s\n", kandevOut)
	return buildKandevDocker(ctx, kandevOut)
}

// buildKandevDocker builds kandev inside a linux/amd64 Docker container.
// apps/backend is mounted at /work; the output is written to /work/bin/kandev
// then copied to the host out path.
func buildKandevDocker(ctx context.Context, out string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	// Ensure the host-side bin/ directory exists before Docker tries to write
	// into it. `go build -o` does not create missing parent directories.
	if err := os.MkdirAll(filepath.Join(wd, "bin"), 0o755); err != nil {
		return fmt.Errorf("mkdir bin: %w", err)
	}
	// Output inside the container (relative to /work mount).
	containerOut := "/work/bin/kandev-preview-build"
	goCache := filepath.Join(os.Getenv("HOME"), ".cache", "go-build-linux")
	goModCache := filepath.Join(os.Getenv("HOME"), "go", "pkg", "mod")

	cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"--platform", "linux/amd64",
		"-v", wd+":/work",
		"-v", goCache+":/root/.cache/go-build",
		"-v", goModCache+":/go/pkg/mod",
		"-w", "/work",
		goDockerImage,
		"go", "build", "-ldflags", "-s -w",
		"-o", containerOut,
		"./cmd/kandev",
	)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker build kandev: %w", err)
	}

	// Copy from temp path (inside /work mount) to the desired output path.
	// os.Rename fails with EXDEV when hostTmp and out are on different filesystems
	// (e.g. the repo is on a regular disk and out is under os.TempDir() on tmpfs).
	hostTmp := filepath.Join(wd, "bin", "kandev-preview-build")
	defer func() { _ = os.Remove(hostTmp) }()
	return copyFile(hostTmp, out, 0o755)
}

func copyFile(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %s → %s: %w", src, dst, err)
	}
	return nil
}

// buildWeb runs the Vite production build. When skipInstall is true the
// pnpm install step is skipped (CI already runs it before invoking the CLI).
func buildWeb(ctx context.Context, skipInstall bool) error {
	steps := []struct {
		desc string
		args []string
	}{
		{"pnpm install", []string{"pnpm", "install", "--frozen-lockfile"}},
		{"pnpm build web", []string{"pnpm", "--filter", "@kandev/web", "build"}},
	}
	for _, step := range steps {
		if skipInstall && step.desc == "pnpm install" {
			continue
		}
		fmt.Fprintf(os.Stderr, "  %s\n", step.desc)
		cmd := exec.CommandContext(ctx, step.args[0], step.args[1:]...)
		cmd.Dir = appsDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s: %w", step.desc, err)
		}
	}
	return nil
}

// buildCLI creates the bundled TypeScript launcher used by release-style
// installs. Preview sprites run through this launcher so their backend is
// managed by the same restart supervisor as normal kandev starts.
func buildCLI(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "bash", "scripts/release/package-cli.sh")
	cmd.Dir = repoRootDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("package cli: %w", err)
	}
	return nil
}

// packageBundle creates a tar.gz matching the Docker container layout:
//
//	app/apps/backend/bin/{kandev,agentctl,mock-agent}
//	app/apps/web/dist/                       (Vite SPA assets)
//	usr/local/lib/kandev-cli/                (CLI launcher bundle)
func packageBundle(binDir, tarPath string) error {
	f, err := os.Create(tarPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	// Add binaries.
	binEntries, err := os.ReadDir(binDir)
	if err != nil {
		return fmt.Errorf("read bin dir: %w", err)
	}
	for _, e := range binEntries {
		src := filepath.Join(binDir, e.Name())
		dst := filepath.Join("app", "apps", "backend", "bin", e.Name())
		if err := addFileToTar(tw, src, dst, 0o755); err != nil {
			return err
		}
	}

	// Add Vite SPA assets. `kandev start` resolves these from /app/apps/web/dist
	// and passes KANDEV_WEB_DIST_DIR to the backend.
	webDistDir := filepath.Join(appsDir, "web", "dist")
	if err := validateViteWebDist(webDistDir); err != nil {
		return err
	}
	if err := addDirToTar(tw, webDistDir, filepath.Join("app", "apps", "web", "dist")); err != nil {
		return fmt.Errorf("add web dist: %w", err)
	}

	cliDir := filepath.Join(repoRootDir, "dist", "kandev", "cli")
	if err := addDirToTar(tw, cliDir, filepath.Join("usr", "local", "lib", "kandev-cli")); err != nil {
		return fmt.Errorf("add cli: %w", err)
	}

	// Close in order: tar → gzip → file (flush compressed data).
	if err := tw.Close(); err != nil {
		return fmt.Errorf("close tar: %w", err)
	}
	return gz.Close()
}

func validateViteWebDist(webDistDir string) error {
	indexPath := filepath.Join(webDistDir, "index.html")
	indexHTML, err := os.ReadFile(indexPath)
	if err != nil {
		return fmt.Errorf("read web dist index: %w", err)
	}
	if !viteIndexHasEntrypoint(string(indexHTML)) {
		return fmt.Errorf("web dist index %s is missing Vite module entrypoint", indexPath)
	}
	return nil
}

func viteIndexHasEntrypoint(indexHTML string) bool {
	return strings.Contains(indexHTML, "<script") &&
		strings.Contains(indexHTML, `type="module"`) &&
		strings.Contains(indexHTML, `src="/assets/`)
}

func addFileToTar(tw *tar.Writer, src, dst string, mode fs.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	hdr := &tar.Header{
		Name: dst,
		Mode: int64(mode),
		Size: int64(len(data)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = tw.Write(data)
	return err
}

func addDirToTar(tw *tar.Writer, srcDir, dstPrefix string) error {
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(dstPrefix, rel)

		info, err := d.Info()
		if err != nil {
			return err
		}

		// Preserve symlinks as-is.
		if info.Mode()&fs.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			hdr := &tar.Header{
				Typeflag: tar.TypeSymlink,
				Name:     dst,
				Linkname: target,
			}
			return tw.WriteHeader(hdr)
		}

		if d.IsDir() {
			return tw.WriteHeader(&tar.Header{
				Typeflag: tar.TypeDir,
				Name:     dst + "/",
				Mode:     0o755,
			})
		}

		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = dst

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()

		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		_, err = io.Copy(tw, f)
		return err
	})
}
