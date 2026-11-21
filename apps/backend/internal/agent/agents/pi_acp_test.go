package agents

import (
	"slices"
	"testing"
)

func TestPiACPRemoteAuthCopiesAuthFileAndOffersAPIKeys(t *testing.T) {
	auth := NewPiACP().RemoteAuth()
	if auth == nil || len(auth.Methods) == 0 {
		t.Fatal("Pi RemoteAuth must declare methods")
	}

	files := auth.Methods[0]
	if files.Type != remoteAuthMethodTypeFiles {
		t.Fatalf("Methods[0].Type = %q, want files", files.Type)
	}
	if files.Label != "" {
		t.Errorf("Label = %q, want none so the UI shows its translated default", files.Label)
	}
	if files.TargetRelDir != ".pi/agent" {
		t.Errorf("TargetRelDir = %q, want .pi/agent", files.TargetRelDir)
	}
	if files.FileConflictPolicy != RemoteAuthFileConflictPolicyMergeJSONObject {
		t.Errorf("FileConflictPolicy = %q, want a JSON object merge that keeps remote-only providers", files.FileConflictPolicy)
	}
	for _, osName := range []string{"darwin", "linux", "windows"} {
		if paths := files.SourceFiles[osName]; !slices.Equal(paths, []string{".pi/agent/auth.json"}) {
			t.Errorf("SourceFiles[%s] = %#v, want only .pi/agent/auth.json", osName, paths)
		}
	}

	var envVars []string
	for _, method := range auth.Methods[1:] {
		if method.Type != "env" {
			t.Fatalf("method %+v: want only env methods after the auth file", method)
		}
		envVars = append(envVars, method.EnvVar)
	}
	want := []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY", "OPENROUTER_API_KEY"}
	if !slices.Equal(envVars, want) {
		t.Errorf("env vars = %v, want %v", envVars, want)
	}
}

func TestPiACPPortableConfigExcludesCredentials(t *testing.T) {
	bundles := NewPiACP().PortableConfig().Bundles
	if len(bundles) != 1 || bundles[0].ID != "pi.config" {
		t.Fatalf("bundles = %+v, want the single pi.config bundle", bundles)
	}
	var targets []string
	for _, file := range bundles[0].Files {
		targets = append(targets, file.TargetPath)
		for osName, source := range file.SourcePaths {
			if source != file.TargetPath {
				t.Errorf("%s source %q differs from target %q", osName, source, file.TargetPath)
			}
		}
	}
	if !slices.Equal(targets, []string{".pi/agent/settings.json", ".pi/agent/models.json"}) {
		t.Errorf("targets = %v", targets)
	}
	if slices.Contains(targets, ".pi/agent/auth.json") {
		t.Error("auth.json belongs to RemoteAuth, not to the configuration bundle")
	}
}
