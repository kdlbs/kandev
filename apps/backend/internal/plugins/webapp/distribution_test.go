package webapp

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestValidateDistributionPackageAcceptsProjectSourceAndChecksums(t *testing.T) {
	files := distributionFiles()
	files["checksums.txt"] = checksumFile(files)
	archive := gzipTarFiles(t, files)
	pkg, err := ValidateDistributionPackage(bytes.NewReader(archive))
	if err != nil {
		t.Fatalf("ValidateDistributionPackage() unexpected error: %v", err)
	}
	if pkg.Manifest.Distribution.SourceMode != "project" {
		t.Fatalf("distribution = %+v", pkg.Manifest.Distribution)
	}
}

func TestValidateDistributionPackageRejectsChecksumAndSourceViolations(t *testing.T) {
	cases := []struct {
		name string
		edit func(map[string]string)
		want error
	}{
		{name: "missing checksum", edit: func(files map[string]string) { delete(files, "checksums.txt") }, want: ErrChecksumMissing},
		{name: "checksum mismatch", edit: func(files map[string]string) { files["ui/app.js"] = "changed" }, want: ErrChecksumMismatch},
		{name: "credential file", edit: func(files map[string]string) { files["distribution/source/.env"] = "TOKEN=secret\n" }, want: ErrUnsafeSource},
		{name: "nested archive", edit: func(files map[string]string) { files["distribution/source/app.zip"] = "PK\x03\x04" }, want: ErrUnsafeSource},
		{name: "missing project source", edit: func(files map[string]string) {
			for name := range files {
				if strings.HasPrefix(name, "distribution/source/") {
					delete(files, name)
				}
			}
		}, want: ErrSourceUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := distributionFiles()
			if tc.name == "checksum mismatch" {
				files["checksums.txt"] = checksumFile(files)
			}
			tc.edit(files)
			if tc.name != "missing checksum" && tc.name != "checksum mismatch" {
				files["checksums.txt"] = checksumFile(files)
			}
			_, err := ValidateDistributionPackage(bytes.NewReader(gzipTarFiles(t, files)))
			if !errors.Is(err, tc.want) {
				t.Fatalf("ValidateDistributionPackage() error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestDistributionArchiveWritersRoundTrip(t *testing.T) {
	pkg := packageFromFiles(t, distributionFiles())
	archives, err := BuildDistributionArchives(pkg)
	if err != nil {
		t.Fatalf("BuildDistributionArchives() unexpected error: %v", err)
	}
	if len(archives.Bundle) == 0 || len(archives.Source) == 0 {
		t.Fatalf("archives = %+v, want bundle and source bytes", archives)
	}
	if _, err := ValidateDistributionPackage(bytes.NewReader(archives.Bundle)); err != nil {
		t.Fatalf("bundle validation error = %v", err)
	}
	if _, err := ValidateSourceArchive(bytes.NewReader(archives.Source)); err != nil {
		t.Fatalf("source validation error = %v", err)
	}
}

func TestRuntimeFilePathRejectsDistributionAliases(t *testing.T) {
	for _, requestPath := range []string{
		"distribution/source/app.ts",
		"distribution%2Fsource/app.ts",
		"ui/../distribution/source/app.ts",
		"ui/%2e%2e/distribution/source/app.ts",
	} {
		if _, err := runtimeFilePath(requestPath, "ui/index.html"); !errors.Is(err, ErrUnsafePath) {
			t.Errorf("runtimeFilePath(%q) error = %v, want ErrUnsafePath", requestPath, err)
		}
	}
}

const distributionManifestYAML = `id: canvas-board
api_version: 2
version: 1.2.3
display_name: Canvas Board
description: A board
author: Kandev
categories: [canvas]
min_kandev_version: 1.0.0
distribution:
  schema_version: 1
  kind: canvas
  license: Custom-License
  source_mode: project
ui:
  web_apps:
    - key: main
      title: Board
      entry: ui/index.html
      placements: [workspace-canvas]
`

func distributionFiles() map[string]string {
	return map[string]string{
		"manifest.yaml":                      distributionManifestYAML,
		"README.md":                          "Build with pnpm build.\n",
		"LICENSE.txt":                        "Custom license terms.\n",
		"ui/index.html":                      "<html></html>",
		"ui/app.js":                          "console.log('ok');",
		"distribution/source/manifest.yaml":  "name: canvas-board\n",
		"distribution/source/README.md":      "Build with pnpm build.\n",
		"distribution/source/package.json":   "{\"scripts\":{\"build\":\"vite build\"}}\n",
		"distribution/source/src/main.tsx":   "export default function Main() { return null; }\n",
		"distribution/source/pnpm-lock.yaml": "lockfileVersion: '9.0'\n",
	}
}

func packageFromFiles(t *testing.T, files map[string]string) *Package {
	t.Helper()
	files["checksums.txt"] = checksumFile(files)
	pkg, err := ValidateDistributionPackage(bytes.NewReader(gzipTarFiles(t, files)))
	if err != nil {
		t.Fatalf("ValidateDistributionPackage() unexpected error: %v", err)
	}
	return pkg
}

func checksumFile(files map[string]string) string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if names[j] < names[i] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	var builder strings.Builder
	for _, name := range names {
		sum := sha256.Sum256([]byte(files[name]))
		builder.WriteString(hex.EncodeToString(sum[:]))
		builder.WriteString("  ")
		builder.WriteString(name)
		builder.WriteByte('\n')
	}
	return builder.String()
}

func gzipTarFiles(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	tw := tar.NewWriter(zw)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if names[j] < names[i] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	for _, name := range names {
		data := []byte(files[name])
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatalf("tar header: %v", err)
		}
		if _, err := io.Copy(tw, bytes.NewReader(data)); err != nil {
			t.Fatalf("tar data: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return compressed.Bytes()
}
