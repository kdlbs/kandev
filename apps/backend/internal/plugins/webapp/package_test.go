package webapp

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidatePackageAcceptsStaticCanvas(t *testing.T) {
	archive := canvasArchive(t, map[string]string{
		"manifest.yaml": `id: canvas-board
api_version: 2
version: 1.0.0
display_name: Canvas Board
description: Static board
author: test
ui:
  web_apps:
    - key: main
      title: Task board
      entry: ui/index.html
      placements: [task-canvas, workspace-canvas]
`,
		"ui/index.html": "<!doctype html><html><body>board</body></html>",
		"ui/app.js":     "document.body.dataset.ready = 'true';",
		"ui/app.css":    "body { color: black; }",
	})

	pkg, err := ValidatePackage(bytes.NewReader(archive))
	if err != nil {
		t.Fatalf("ValidatePackage() unexpected error: %v", err)
	}
	if pkg.Manifest.ID != "canvas-board" || pkg.Manifest.UI.WebApps[0].Entry != "ui/index.html" {
		t.Fatalf("validated manifest = %+v", pkg.Manifest)
	}
	if pkg.Digest == "" || pkg.ExpandedBytes == 0 || pkg.CompressedBytes != int64(len(archive)) {
		t.Fatalf("package metadata = %+v", pkg)
	}
}

func TestValidatePackageRejectsTraversalSymlinkDuplicateAndUnsupported(t *testing.T) {
	cases := []struct {
		name  string
		entry tar.Header
		want  error
	}{
		{name: "traversal", entry: tar.Header{Name: "../escape.js", Mode: 0o600, Size: 1, Typeflag: tar.TypeReg}, want: ErrUnsafePath},
		{name: "symlink", entry: tar.Header{Name: "ui/link.js", Linkname: "index.html", Typeflag: tar.TypeSymlink}, want: ErrUnsafeEntry},
		{name: "unsupported", entry: tar.Header{Name: "ui/app.wasm", Mode: 0o600, Size: 1, Typeflag: tar.TypeReg}, want: ErrUnsupportedFile},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			archive := archiveWithHeaders(t, []archiveEntry{
				{header: tar.Header{Name: "manifest.yaml", Mode: 0o600, Size: int64(len(staticManifestYAML)), Typeflag: tar.TypeReg}, data: staticManifestYAML},
				{header: tc.entry, data: "x"},
			})
			_, err := ValidatePackage(bytes.NewReader(archive))
			if !errors.Is(err, tc.want) {
				t.Fatalf("ValidatePackage() error = %v, want %v", err, tc.want)
			}
		})
	}

	duplicate := archiveWithHeaders(t, []archiveEntry{
		{header: tar.Header{Name: "manifest.yaml", Mode: 0o600, Size: int64(len(staticManifestYAML)), Typeflag: tar.TypeReg}, data: staticManifestYAML},
		{header: tar.Header{Name: "ui/index.html", Mode: 0o600, Size: 1, Typeflag: tar.TypeReg}, data: "x"},
		{header: tar.Header{Name: "ui/./index.html", Mode: 0o600, Size: 1, Typeflag: tar.TypeReg}, data: "y"},
	})
	if _, err := ValidatePackage(bytes.NewReader(duplicate)); !errors.Is(err, ErrDuplicatePath) {
		t.Fatalf("duplicate error = %v, want %v", err, ErrDuplicatePath)
	}
}

func TestValidatePackageRejectsOversizedFileWithoutRetainingIt(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxFileBytes = 4
	archive := canvasArchive(t, map[string]string{
		"manifest.yaml": staticManifestYAML,
		"ui/index.html": "12345",
	})
	_, err := ValidatePackageWithLimits(bytes.NewReader(archive), limits)
	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("ValidatePackageWithLimits() error = %v, want %v", err, ErrFileTooLarge)
	}
}

func TestValidatePackageRejectsManagedWebAppPackage(t *testing.T) {
	archive := canvasArchive(t, map[string]string{
		"manifest.yaml": `id: managed-board
api_version: 2
version: 1.0.0
display_name: Managed Board
description: Managed web app
author: test
base_url: https://plugin.example.test
endpoints:
  health: /health
  events: /events
  webhooks: /webhooks
ui:
  web_apps:
    - key: main
      title: Board
      entry: ui/index.html
      placements: [task-canvas]
`,
		"ui/index.html": "<html>managed</html>",
	})
	if _, err := ValidatePackage(bytes.NewReader(archive)); !errors.Is(err, ErrNotStaticWebApp) {
		t.Fatalf("ValidatePackage() error = %v, want %v", err, ErrNotStaticWebApp)
	}
}

func TestArtifactStorePutAndReconcileMarksChangedArtifactUnavailable(t *testing.T) {
	archive := canvasArchive(t, map[string]string{
		"manifest.yaml": staticManifestYAML,
		"ui/index.html": "<html>ok</html>",
	})
	pkg, err := ValidatePackage(bytes.NewReader(archive))
	if err != nil {
		t.Fatalf("ValidatePackage() unexpected error: %v", err)
	}
	store, err := NewArtifactStore(filepath.Join(t.TempDir(), "artifacts"))
	if err != nil {
		t.Fatalf("NewArtifactStore() unexpected error: %v", err)
	}
	artifact, err := store.Put(pkg)
	if err != nil {
		t.Fatalf("Put() unexpected error: %v", err)
	}
	if !artifact.Available || artifact.RelativePath == "" {
		t.Fatalf("artifact = %+v", artifact)
	}
	if err := os.WriteFile(filepath.Join(store.Path(artifact), "ui", "index.html"), []byte("tampered"), 0o600); err != nil {
		t.Fatalf("tamper artifact: %v", err)
	}
	status, err := store.Reconcile(artifact)
	if err != nil {
		t.Fatalf("Reconcile() unexpected error: %v", err)
	}
	if status.Available || status.Reason != "digest_mismatch" {
		t.Fatalf("reconciled artifact = %+v", status)
	}
}

const staticManifestYAML = `id: canvas-board
api_version: 2
version: 1.0.0
display_name: Canvas Board
description: Static board
author: test
ui:
  web_apps:
    - key: main
      title: Task board
      entry: ui/index.html
      placements: [task-canvas]
`

type archiveEntry struct {
	header tar.Header
	data   string
}

func canvasArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	entries := make([]archiveEntry, 0, len(files))
	for name, data := range files {
		entries = append(entries, archiveEntry{header: tar.Header{Name: name, Mode: 0o600, Size: int64(len(data)), Typeflag: tar.TypeReg}, data: data})
	}
	return archiveWithHeaders(t, entries)
}

func archiveWithHeaders(t *testing.T, entries []archiveEntry) []byte {
	t.Helper()
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	tw := tar.NewWriter(zw)
	for _, entry := range entries {
		if err := tw.WriteHeader(&entry.header); err != nil {
			t.Fatalf("WriteHeader() unexpected error: %v", err)
		}
		if entry.header.Typeflag == tar.TypeReg {
			if _, err := io.Copy(tw, strings.NewReader(entry.data)); err != nil {
				t.Fatalf("write archive data: %v", err)
			}
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
