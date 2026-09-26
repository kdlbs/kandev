package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
)

type recordingPortableConfigUploader struct {
	writes []portableConfigWrite
}

type portableConfigWrite struct {
	path string
	data []byte
	mode os.FileMode
}

func (u *recordingPortableConfigUploader) WriteFile(_ context.Context, path string, data []byte, mode os.FileMode) error {
	u.writes = append(u.writes, portableConfigWrite{path: path, data: append([]byte(nil), data...), mode: mode})
	return nil
}

type portableConfigTestAgent struct {
	agents.Agent
	config *agents.PortableConfig
}

func (a portableConfigTestAgent) PortableConfig() *agents.PortableConfig { return a.config }

func TestUploadPortableConfigBundlesCopiesSelectedFilesWithPrivateMode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeFile(t, home, ".agent/settings.json", []byte("settings"))
	writeFile(t, home, ".agent/optional.json", []byte("optional"))

	ag := portableConfigTestAgent{
		Agent: agents.NewMockAgent(),
		config: &agents.PortableConfig{Bundles: []agents.PortableConfigBundle{{
			ID: "test.config", Label: "Test config", Files: []agents.PortableConfigFile{
				{SourcePaths: map[string]string{"linux": ".agent/settings.json"}, TargetPath: ".agent/settings.json"},
				{SourcePaths: map[string]string{"linux": ".agent/optional.json"}, TargetPath: ".agent/optional.json"},
			},
		}}},
	}
	uploader := &recordingPortableConfigUploader{}

	warnings := UploadPortableConfigBundles(
		context.Background(), uploader, ag, []string{"test.config"}, filepath.Join(t.TempDir(), "remote-home"), newSeederTestLogger(t),
	)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %+v, want none", warnings)
	}
	if len(uploader.writes) != 2 {
		t.Fatalf("writes = %+v, want two selected files", uploader.writes)
	}
	for _, write := range uploader.writes {
		if write.mode != 0o600 {
			t.Errorf("mode for %s = %o, want 600", write.path, write.mode)
		}
	}
}

func TestUploadPortableConfigBundlesRejectsUnsafeAndOversizedFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeFile(t, home, ".agent/link.json", []byte("link"))
	if err := os.Symlink(filepath.Join(home, ".agent", "link.json"), filepath.Join(home, ".agent", "symlink.json")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	writeFile(t, home, ".agent/large.json", make([]byte, portableConfigMaxFileBytes+1))

	ag := portableConfigTestAgent{
		Agent: agents.NewMockAgent(),
		config: &agents.PortableConfig{Bundles: []agents.PortableConfigBundle{{
			ID: "unsafe.config", Label: "Unsafe config", Files: []agents.PortableConfigFile{
				{SourcePaths: map[string]string{"linux": ".agent/symlink.json"}, TargetPath: ".agent/symlink.json"},
				{SourcePaths: map[string]string{"linux": ".agent/large.json"}, TargetPath: ".agent/large.json"},
				{SourcePaths: map[string]string{"linux": ".agent/link.json"}, TargetPath: "../escape.json"},
			},
		}}},
	}
	uploader := &recordingPortableConfigUploader{}
	warnings := UploadPortableConfigBundles(
		context.Background(), uploader, ag, []string{"unsafe.config"}, filepath.Join(t.TempDir(), "remote-home"), newSeederTestLogger(t),
	)
	if len(uploader.writes) != 0 {
		t.Fatalf("unsafe files were copied: %+v", uploader.writes)
	}
	if len(warnings) != 3 {
		t.Fatalf("warnings = %+v, want one for each rejected file", warnings)
	}
}

func TestUploadPortableConfigBundlesWarnsAndContinuesForMissingFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeFile(t, home, ".agent/present.json", []byte("present"))
	ag := portableConfigTestAgent{
		Agent: agents.NewMockAgent(),
		config: &agents.PortableConfig{Bundles: []agents.PortableConfigBundle{{
			ID: "partial.config", Label: "Partial config", Files: []agents.PortableConfigFile{
				{SourcePaths: map[string]string{"linux": ".agent/present.json"}, TargetPath: ".agent/present.json"},
				{SourcePaths: map[string]string{"linux": ".agent/missing.json"}, TargetPath: ".agent/missing.json"},
			},
		}}},
	}
	uploader := &recordingPortableConfigUploader{}
	warnings := UploadPortableConfigBundles(
		context.Background(), uploader, ag, []string{"partial.config"}, filepath.Join(t.TempDir(), "remote-home"), newSeederTestLogger(t),
	)
	if len(uploader.writes) != 1 {
		t.Fatalf("writes = %+v, want present file copied", uploader.writes)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %+v, want missing-file warning", warnings)
	}
}

func TestReadPortableConfigFileRejectsPathReplacement(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "settings.json")
	writeFile(t, root, "settings.json", []byte("original"))
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat source: %v", err)
	}
	replacement := filepath.Join(root, "replacement.json")
	writeFile(t, root, "replacement.json", []byte("replacement"))
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove source: %v", err)
	}
	if err := os.Symlink(replacement, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, err := readPortableConfigFile(path, info); err == nil {
		t.Fatal("expected replaced source path to be rejected")
	}
}

func TestUploadPortableConfigBundlesRejectsSymlinkedDestinations(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeFile(t, home, ".agent/settings.json", []byte("settings"))

	for _, tc := range []struct {
		name       string
		targetPath string
		prepare    func(t *testing.T, root, outside string) string
	}{
		{
			name:       "final file",
			targetPath: ".agent/settings.json",
			prepare: func(t *testing.T, root, outside string) string {
				t.Helper()
				target := filepath.Join(root, ".agent", "settings.json")
				if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
					t.Fatalf("mkdir target parent: %v", err)
				}
				outsideTarget := filepath.Join(outside, "settings.json")
				writeFile(t, outside, "settings.json", []byte("outside"))
				if err := os.Symlink(outsideTarget, target); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
				return outsideTarget
			},
		},
		{
			name:       "parent directory",
			targetPath: ".redirect/settings.json",
			prepare: func(t *testing.T, root, outside string) string {
				t.Helper()
				parent := filepath.Join(root, ".redirect")
				outsideTarget := filepath.Join(outside, "settings.json")
				writeFile(t, outside, "settings.json", []byte("outside"))
				if err := os.Symlink(outside, parent); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
				return outsideTarget
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			outside := t.TempDir()
			outsideTarget := tc.prepare(t, root, outside)
			ag := portableConfigTestAgent{
				Agent: agents.NewMockAgent(),
				config: &agents.PortableConfig{Bundles: []agents.PortableConfigBundle{{
					ID: "test.config", Label: "Test config", Files: []agents.PortableConfigFile{{
						SourcePaths: map[string]string{
							"linux":   ".agent/settings.json",
							"darwin":  ".agent/settings.json",
							"windows": ".agent/settings.json",
						},
						TargetPath: tc.targetPath,
					}},
				}}},
			}

			warnings := UploadPortableConfigBundles(
				context.Background(), localFileUploader{root: root}, ag,
				[]string{"test.config"}, root, newSeederTestLogger(t),
			)
			if len(warnings) != 1 || warnings[0].Reason != "target_write_failed" {
				t.Fatalf("warnings = %+v, want target_write_failed", warnings)
			}
			contents, err := os.ReadFile(outsideTarget)
			if err != nil {
				t.Fatalf("read outside target: %v", err)
			}
			if string(contents) != "outside" {
				t.Fatalf("outside target = %q, want unchanged content", contents)
			}
		})
	}
}

func TestReportPortableConfigWarningsPublishesOptionalCopyWarning(t *testing.T) {
	var got PrepareStep
	reportPortableConfigWarnings(func(step PrepareStep, _, _ int) {
		got = step
	}, []PortableConfigWarning{
		{BundleID: "test.config", SourcePath: ".agent/missing.json", Reason: "source_missing"},
		{BundleID: "test.config", SourcePath: ".agent/large.json", Reason: "file_too_large"},
	})

	if got.Name != "Copy agent configuration" {
		t.Fatalf("step name = %q, want copy step", got.Name)
	}
	if got.Status != PrepareStepCompleted {
		t.Fatalf("step status = %q, want completed", got.Status)
	}
	if got.Warning == "" || got.WarningDetail == "" {
		t.Fatalf("step = %+v, want warning and warning detail", got)
	}
	if got.Output != "" {
		t.Fatalf("step output = %q, must not contain copied file data", got.Output)
	}
}
