package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"runtime"
	"testing"

	"github.com/kandev/kandev/internal/plugins/pkgtar/pkgtartest"
)

func TestRunInspectsPluginPackageAsSafeJSON(t *testing.T) {
	platform := runtime.GOOS + "-" + runtime.GOARCH
	files := map[string][]byte{
		"manifest.yaml": []byte(fmt.Sprintf(`
id: example-plugin
api_version: 1
version: 1.2.3
display_name: Example Plugin
description: A plugin package
author: Example contributors
categories: [tools, analytics]
icon: assets/icon.svg
repo_url: https://github.com/acme/example-plugin
runtime:
  type: binary
  executables:
    %s: server/plugin-%s
`, platform, platform)),
		"server/plugin-" + platform: []byte("binary"),
		"assets/icon.svg":           []byte("<svg></svg>"),
	}
	var archive bytes.Buffer
	if err := pkgtartest.WritePackage(&archive, files); err != nil {
		t.Fatalf("WritePackage() error: %v", err)
	}

	var output, errorOutput bytes.Buffer
	code := run([]string{"--file", "-"}, bytes.NewReader(archive.Bytes()), &output, &errorOutput)
	if code != 0 {
		t.Fatalf("run() exit code = %d, stderr = %q", code, errorOutput.String())
	}
	var descriptor packageDescriptor
	if err := json.Unmarshal(output.Bytes(), &descriptor); err != nil {
		t.Fatalf("decode descriptor: %v", err)
	}
	if descriptor.ID != "example-plugin" || descriptor.Version != "1.2.3" {
		t.Fatalf("descriptor identity = %q@%q", descriptor.ID, descriptor.Version)
	}
	if descriptor.Author != "Example contributors" || len(descriptor.Categories) != 2 {
		t.Fatalf("descriptor metadata = %+v", descriptor)
	}
	if len(descriptor.Files) != 4 {
		t.Fatalf("descriptor package evidence = %+v", descriptor)
	}
}

func TestRunRejectsInvalidPluginPackageWithoutLeakingDetails(t *testing.T) {
	var output, errorOutput bytes.Buffer
	code := run([]string{"--file", "-"}, bytes.NewReader([]byte("not an archive")), &output, &errorOutput)
	if code == 0 || output.Len() != 0 {
		t.Fatalf("run() code=%d output=%q", code, output.String())
	}
	if bytes.Contains(errorOutput.Bytes(), []byte("/")) {
		t.Fatalf("error output contains a path: %q", errorOutput.String())
	}
}
