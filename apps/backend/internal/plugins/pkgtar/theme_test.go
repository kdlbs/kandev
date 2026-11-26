package pkgtar

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/plugins/pkgtar/pkgtartest"
)

func TestInspectBoundsExpansionBeforeLateManifest(t *testing.T) {
	previous := maxPackageTotalSize
	maxPackageTotalSize = 1024
	t.Cleanup(func() { maxPackageTotalSize = previous })
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, file := range []struct {
		name string
		data []byte
	}{{"padding", make([]byte, 2<<20)}, {manifestFileName, themeFiles()[manifestFileName]}} {
		if err := tw.WriteHeader(&tar.Header{Name: file.name, Mode: 0600, Size: int64(len(file.data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(file.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(&buf); err == nil {
		t.Fatal("accepted excessive expansion before manifest")
	}
}

func themeFiles() map[string][]byte {
	return map[string][]byte{
		"manifest.yaml": []byte(`id: sample-themes
api_version: 2
version: 1.0.0
display_name: Sample themes
distribution: {schema_version: 1, kind: theme}
themes:
- {key: dark, label: Dark, base: dark, path: themes/dark.json}
- {key: light, label: Light, base: light, path: themes/light.json}
`),
		"themes/dark.json":  []byte(`{"schema_version":1,"ui":{"background":"#272822"}}`),
		"themes/light.json": []byte(`{"schema_version":1,"ui":{"background":"#fff"}}`),
	}
}

// @covers AC-PLUGINS-THEMES-001.1
func TestThemeArchiveVerifyAndInstall(t *testing.T) {
	var buf bytes.Buffer
	if err := pkgtartest.WritePackage(&buf, themeFiles()); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatal(err)
	}
	result, err := Install(bytes.NewReader(buf.Bytes()), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(result.InstallPath, "themes/dark.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0111 != 0 {
		t.Fatal("theme extracted executable")
	}
}

func TestThemeArchiveRejectsUndeclaredAndInvalidData(t *testing.T) {
	for name, data := range map[string]string{"ui/bundle.js": "alert(1)", "themes/dark.json": `{"schema_version":1,"ui":{"background":"red"}}`, "README": string(make([]byte, 65537))} {
		files := themeFiles()
		files[name] = []byte(data)
		var buf bytes.Buffer
		if err := pkgtartest.WritePackage(&buf, files); err != nil {
			t.Fatal(err)
		}
		if _, err := Verify(&buf); err == nil {
			t.Fatalf("accepted %s", name)
		}
	}
}

func TestThemeDirectoryRejectsTampering(t *testing.T) {
	var buf bytes.Buffer
	if err := pkgtartest.WritePackage(&buf, themeFiles()); err != nil {
		t.Fatal(err)
	}
	result, err := Install(&buf, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadThemeDirectory(result.InstallPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(result.InstallPath, "themes/dark.json"), []byte(`{"schema_version":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadThemeDirectory(result.InstallPath); err == nil {
		t.Fatal("tampered install accepted")
	}
}

func TestThemeArchiveBoundsBeforeExtraction(t *testing.T) {
	files := themeFiles()
	files["themes/dark.json"] = append([]byte(`{"schema_version":1}`), bytes.Repeat([]byte(" "), 256<<10)...)
	var buf bytes.Buffer
	if err := pkgtartest.WritePackage(&buf, files); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(&buf, t.TempDir()); err == nil {
		t.Fatal("oversized definition accepted")
	}
}
