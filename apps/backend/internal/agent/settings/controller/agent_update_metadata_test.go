package controller

import (
	"context"
	"testing"
)

func TestHostRuntimeUpdaterResolvesNPM12ArrayVersionCatalogue(t *testing.T) {
	executor := &recordingCommandExecutor{
		output: `[{"versions":["1.0.1","1.0.2"],"dist-tags":{"latest":"1.0.2"}}]`,
	}
	updater := &hostRuntimeUpdater{executor: executor}

	metadata, err := updater.ResolveVersions(context.Background(), "@example/managed-acp")
	if err != nil {
		t.Fatalf("ResolveVersions: %v", err)
	}
	if metadata.Latest != "1.0.2" || len(metadata.Versions) != 2 {
		t.Fatalf("metadata = %#v", metadata)
	}
}

func TestHostRuntimeUpdaterRejectsAmbiguousNPMRuntimeMetadata(t *testing.T) {
	tests := map[string]string{
		"empty array":       `[]`,
		"multiple records":  `[{"dist-tags":{"latest":"1.0.1"}},{"dist-tags":{"latest":"1.0.2"}}]`,
		"malformed payload": `{not-json`,
	}

	for name, output := range tests {
		t.Run(name, func(t *testing.T) {
			updater := &hostRuntimeUpdater{executor: &recordingCommandExecutor{output: output}}
			if _, err := updater.ResolveVersions(context.Background(), "@example/managed-acp"); err == nil {
				t.Fatal("ResolveVersions succeeded, want metadata error")
			}
		})
	}
}
