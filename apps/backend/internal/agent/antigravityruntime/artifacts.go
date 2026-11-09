// Package antigravityruntime owns the reviewed Antigravity ACP server bundle:
// trusted artifact records, bounded installation, filesystem validation, and
// local resolution. It is not a generic native package manager.
package antigravityruntime

import (
	"errors"
	"fmt"
	"runtime"
)

// Bundle version and archive identity reviewed for this release.
const (
	Version = "1.4.5"

	windowsOS          = "windows"
	maxCompressedBytes = int64(512 << 20)
	maxExpandedBytes   = int64(1 << 30)
)

var (
	// ErrUnavailable means no complete managed or manual runtime exists.
	ErrUnavailable = errors.New("antigravity ACP runtime is not installed")
	// ErrUnsupportedPlatform means the host has no reviewed artifact.
	ErrUnsupportedPlatform = errors.New("antigravity ACP runtime is unsupported on this platform")
	// ErrInvalidManagedBundle means a managed destination exists but is incomplete
	// or inconsistent. It must fail visibly instead of falling back to PATH.
	ErrInvalidManagedBundle = errors.New("antigravity ACP managed bundle is invalid")
	// ErrInvalidHarnessOverride means ANTIGRAVITY_HARNESS_PATH does not name a
	// usable regular executable.
	ErrInvalidHarnessOverride = errors.New("ANTIGRAVITY_HARNESS_PATH is invalid")
	// ErrUnsafeArchive means a downloaded archive failed validation.
	ErrUnsafeArchive = errors.New("antigravity ACP archive is unsafe")
	// ErrDigestMismatch means the downloaded archive did not match the pin.
	ErrDigestMismatch = errors.New("antigravity ACP archive digest mismatch")
	// ErrSizeLimit means an archive or expanded file exceeded its bound.
	ErrSizeLimit = errors.New("antigravity ACP archive exceeds its size limit")
)

// Artifact is one reviewed, immutable platform bundle.
type Artifact struct {
	Version         string
	OS              string
	Arch            string
	URL             string
	SHA256          string
	CompressedLimit int64
	ExpandedLimit   int64
	ServerName      string
	HarnessNames    []string
}

// Artifacts returns the reviewed artifact table. URLs are never accepted from
// requests; installation selects exactly one entry for the host platform.
func Artifacts() []Artifact {
	return []Artifact{
		artifact("darwin", "arm64",
			"https://dl.google.com/agy-extensions/releases/macos/agy-acp-server-1.4.5-darwin-arm64.zip",
			"886fff7767d08994ca39eb02c3d01a7532c7325a94aa33aaf52d52068733284f"),
		artifact("darwin", "amd64",
			"https://dl.google.com/agy-extensions/releases/macos/agy-acp-server-1.4.5-darwin-x86_64.zip",
			"b39f3573c03b9acb9df028853f647527f1f3133f32bd77e78b796e48a205114a"),
		artifact("linux", "amd64",
			"https://dl.google.com/agy-extensions/releases/linux/agy-acp-server-1.4.5-linux-x86_64.zip",
			"923923a0d7dc8073f66cc6f78647a3107e3a7ee2b7e1a24df8b3927b5188013b"),
		artifact("linux", "arm64",
			"https://dl.google.com/agy-extensions/releases/linux/agy-acp-server-1.4.5-linux-arm64.zip",
			"7d64246633ae094f67e3bbcd2d679e89048f91346d5339313336e7483f40f056"),
		artifact(windowsOS, "amd64",
			"https://dl.google.com/agy-extensions/releases/windows/agy-acp-server-1.4.5-windows-x86_64.zip",
			"d24f4c73c9f96d4f44384e281c855a56c72d8eaa9c5b0fad7ccb1635c6b63f50"),
		artifact(windowsOS, "arm64",
			"https://dl.google.com/agy-extensions/releases/windows/agy-acp-server-1.4.5-windows-arm64.zip",
			"ab0a9aac8349e08ef289be6d16da8f89e3abe60eba5158032ef78c4a1220261c"),
	}
}

func artifact(osName, arch, url, digest string) Artifact {
	server := "agy_acp_server.par"
	harness := "localharness_external"
	if osName == windowsOS {
		server = "agy_acp_server.exe"
		harness = "localharness_external.exe"
	}
	return Artifact{
		Version:         Version,
		OS:              osName,
		Arch:            arch,
		URL:             url,
		SHA256:          digest,
		CompressedLimit: maxCompressedBytes,
		ExpandedLimit:   maxExpandedBytes,
		ServerName:      server,
		HarnessNames:    []string{harness},
	}
}

// ArtifactFor selects the reviewed artifact for a platform.
func ArtifactFor(goos, goarch string) (Artifact, error) {
	for _, item := range Artifacts() {
		if item.OS == goos && item.Arch == goarch {
			return item, nil
		}
	}
	return Artifact{}, fmt.Errorf("%w: %s/%s", ErrUnsupportedPlatform, goos, goarch)
}

// HostArtifact selects the reviewed artifact for the running host.
func HostArtifact() (Artifact, error) {
	return ArtifactFor(runtime.GOOS, runtime.GOARCH)
}
