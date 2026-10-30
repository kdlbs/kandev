package plugins

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/plugins/pkgtar/pkgtartest"
)

func testThemeArchive(t *testing.T) *bytes.Reader {
	t.Helper()
	files := map[string][]byte{"manifest.yaml": []byte(`id: sample-themes
api_version: 2
version: 1.0.0
display_name: Sample themes
distribution: {schema_version: 1, kind: theme}
themes: [{key: dark, label: Dark, base: dark, path: dark.json}]
`), "dark.json": []byte(`{"schema_version":1,"ui":{"background":"#272822"}}`)}
	var b bytes.Buffer
	if err := pkgtartest.WritePackage(&b, files); err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(b.Bytes())
}

// @covers AC-PLUGINS-THEMES-003.3
func TestThemeLifecycleNeverStartsRuntime(t *testing.T) {
	svc, _, rt := newTestService(t)
	// Positive control: the same runtime wiring starts an ordinary plugin.
	installTestPlugin(t, svc, "runtime-control")
	before := len(rt.startCalls)
	rec, err := svc.Install(context.Background(), testThemeArchive(t))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != StatusActive {
		t.Fatalf("status %s", rec.Status)
	}
	if err := svc.Disable(rec.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Enable(rec.ID); err != nil {
		t.Fatal(err)
	}
	svc.StartActivePlugins(context.Background())
	if err := svc.Uninstall(context.Background(), rec.ID); err != nil {
		t.Fatal(err)
	}
	if len(rt.startCalls) != before {
		t.Fatalf("theme spawned runtime: %v", rt.startCalls)
	}
	for _, id := range rt.stopCalls {
		if id == rec.ID {
			t.Fatal("theme invoked runtime stop")
		}
	}
}

func TestThemeRestoreRevalidatesAndRevokesTamperedFiles(t *testing.T) {
	svc, _, _ := newTestService(t)
	rec, err := svc.Install(context.Background(), testThemeArchive(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(svc.ThemeInventory().Themes) != 1 {
		t.Fatal("theme inventory missing")
	}
	if err = os.WriteFile(filepath.Join(rec.InstallPath, "dark.json"), []byte(`{"schema_version":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	svc.StartActivePlugins(context.Background())
	if len(svc.ThemeInventory().Themes) != 0 {
		t.Fatal("tampered theme remained eligible")
	}
}
