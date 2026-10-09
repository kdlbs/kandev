package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestMockProfileCatalogUsesEnvironmentAndOrderedCLIFlag(t *testing.T) {
	t.Setenv("MOCK_AGENT_PROFILE_CATALOG", "env-catalog")
	previousArgs := os.Args
	os.Args = []string{"mock-agent", "--profile-order=first", "--profile-catalog=cli-catalog", "--profile-order=last"}
	t.Cleanup(func() { os.Args = previousArgs })

	models := modelIDs(findModelOption(t))
	for _, want := range []string{"profile-env-env-catalog", "profile-cli-cli-catalog"} {
		if !containsString(models, want) {
			t.Errorf("catalog models = %v, missing %q", models, want)
		}
	}
	if got := profileCatalogArgs(os.Args); !reflect.DeepEqual(got, []string{"--profile-catalog=cli-catalog"}) {
		t.Fatalf("captured profile args = %v", got)
	}
}

func TestMockProfileProbeWrapperRecordsInvocationAndForwardsIO(t *testing.T) {
	tempDir := t.TempDir()
	evidencePath := filepath.Join(tempDir, "probe-evidence.jsonl")
	childOutputPath := filepath.Join(tempDir, "child-output")

	exitCode := runProfileProbeWrapper(
		[]string{"/bin/sh", "-c", "printf ran > \"$1\"", "profile-wrapper-test", childOutputPath},
		evidencePath,
	)
	if exitCode != 0 {
		t.Fatalf("wrapper exit code = %d", exitCode)
	}
	if contents, err := os.ReadFile(childOutputPath); err != nil || string(contents) != "ran" {
		t.Fatalf("child output = %q, err = %v", contents, err)
	}

	file, err := os.Open(evidencePath)
	if err != nil {
		t.Fatalf("open evidence: %v", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Errorf("close evidence: %v", err)
		}
	}()
	var evidence profileProbeEvidence
	if err := json.NewDecoder(file).Decode(&evidence); err != nil {
		t.Fatalf("decode evidence: %v", err)
	}
	if !evidence.Wrapper || evidence.Command != "sh" {
		t.Fatalf("evidence = %#v", evidence)
	}
}
