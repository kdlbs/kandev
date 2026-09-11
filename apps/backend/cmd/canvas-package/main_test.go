package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"testing"
)

func TestRunInspectsDistributionPackageAsSafeJSON(t *testing.T) {
	archive := testDistributionArchive(t)
	var output bytes.Buffer
	code := run([]string{"--file", "-"}, bytes.NewReader(archive), &output, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("run() exit code = %d", code)
	}
	var descriptor packageDescriptor
	if err := json.Unmarshal(output.Bytes(), &descriptor); err != nil {
		t.Fatalf("decode descriptor: %v", err)
	}
	if descriptor.ID != "canvas-board" || descriptor.Version != "1.2.3" || descriptor.Kind != "canvas" {
		t.Fatalf("descriptor = %+v", descriptor)
	}
	if descriptor.Digest == "" || len(descriptor.Files) == 0 {
		t.Fatalf("descriptor lacks package metadata: %+v", descriptor)
	}
}

func TestRunRejectsInvalidDistributionWithoutLeakingDetails(t *testing.T) {
	var output, errorOutput bytes.Buffer
	code := run([]string{"--file", "-"}, bytes.NewReader([]byte("not an archive")), &output, &errorOutput)
	if code == 0 || output.Len() != 0 {
		t.Fatalf("run() code=%d output=%q", code, output.String())
	}
	if bytes.Contains(errorOutput.Bytes(), []byte("/")) {
		t.Fatalf("error output contains a path: %q", errorOutput.String())
	}
}

func testDistributionArchive(t *testing.T) []byte {
	t.Helper()
	files := map[string]string{
		"manifest.yaml": `id: canvas-board
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
  license: MIT
  source_mode: static
ui:
  web_apps:
    - key: main
      title: Board
      entry: ui/index.html
      placements: [workspace-canvas]
`,
		"README.md":     "A board.\n",
		"ui/index.html": "<html></html>",
		"checksums.txt": "",
	}
	files["checksums.txt"] = checksumFile(files)
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	tw := tar.NewWriter(zw)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
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

func checksumFile(files map[string]string) string {
	names := make([]string, 0, len(files))
	for name := range files {
		if name != "checksums.txt" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	var output string
	for _, name := range names {
		sum := sha256.Sum256([]byte(files[name]))
		output += fmt.Sprintf("%x  %s\n", sum, name)
	}
	return output
}
