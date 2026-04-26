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
	"runtime"
)

// appsDir is the apps/ workspace root relative to the working directory (apps/backend/).
const appsDir = ".."

// goDockerImage is used to cross-compile CGO binaries on non-linux/amd64 hosts.
const goDockerImage = "golang:1.26-bookworm"

// buildLinuxBinaries compiles kandev, agentctl, and mock-agent for linux/amd64.
// Run this from apps/backend/. agentctl and mock-agent use CGO_ENABLED=0 and
// always build natively. kandev requires CGO (SQLite); on non-linux/amd64 hosts
// it is built inside a Docker container automatically.
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
		fmt.Printf("  go build %s -> %s\n", b.pkg, out)
		cmd := exec.CommandContext(ctx, "go", "build", "-ldflags", "-s -w", "-o", out, b.pkg)
		cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("build %s: %w", b.name, err)
		}
	}

	// kandev requires CGO for SQLite. Build natively on linux/amd64; use Docker otherwise.
	kandevOut := filepath.Join(outDir, "kandev")
	fmt.Printf("  go build ./cmd/kandev -> %s\n", kandevOut)
	if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
		return buildKandevNative(ctx, kandevOut)
	}
	return buildKandevDocker(ctx, kandevOut)
}

func buildKandevNative(ctx context.Context, out string) error {
	cmd := exec.CommandContext(ctx, "go", "build", "-ldflags", "-s -w", "-o", out, "./cmd/kandev")
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=1")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

// buildKandevDocker builds kandev inside a linux/amd64 Docker container.
// apps/backend is mounted at /work; the output is written to /work/bin/kandev
// then copied to the host out path.
func buildKandevDocker(ctx context.Context, out string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
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

	// Move from the temp path (inside /work) to the desired output path.
	hostTmp := filepath.Join(wd, "bin", "kandev-preview-build")
	return os.Rename(hostTmp, out)
}

// buildWeb installs frontend dependencies and runs the Next.js production build.
func buildWeb(ctx context.Context) error {
	for _, step := range []struct {
		desc string
		args []string
	}{
		{"pnpm install", []string{"pnpm", "install", "--frozen-lockfile"}},
		{"pnpm build web", []string{"pnpm", "--filter", "@kandev/web", "build"}},
	} {
		fmt.Printf("  %s\n", step.desc)
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

// packageBundle creates a tar.gz matching the Docker container layout:
//
//	app/apps/backend/bin/{kandev,agentctl,mock-agent}
//	app/apps/web/.next/standalone/           (Next.js server + node_modules)
//	app/apps/web/.next/standalone/web/.next/static/  (static assets inside standalone)
//	app/apps/web/.next/standalone/web/public/        (public assets inside standalone)
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

	// Add Next.js standalone output.
	standaloneDir := filepath.Join(appsDir, "web", ".next", "standalone")
	if err := addDirToTar(tw, standaloneDir, filepath.Join("app", "apps", "web", ".next", "standalone")); err != nil {
		return fmt.Errorf("add standalone: %w", err)
	}

	// Add static assets into the standalone web dir (mirrors the Dockerfile COPY).
	staticDir := filepath.Join(appsDir, "web", ".next", "static")
	staticDst := filepath.Join("app", "apps", "web", ".next", "standalone", "web", ".next", "static")
	if err := addDirToTar(tw, staticDir, staticDst); err != nil {
		return fmt.Errorf("add static: %w", err)
	}

	// Add public directory into the standalone web dir.
	publicDir := filepath.Join(appsDir, "web", "public")
	publicDst := filepath.Join("app", "apps", "web", ".next", "standalone", "web", "public")
	if err := addDirToTar(tw, publicDir, publicDst); err != nil {
		return fmt.Errorf("add public: %w", err)
	}

	// Close in order: tar → gzip → file (flush compressed data).
	if err := tw.Close(); err != nil {
		return fmt.Errorf("close tar: %w", err)
	}
	return gz.Close()
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

		// Handle symlinks by following them.
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
