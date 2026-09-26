package lifecycle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadRemoteHelperManifestValidatesIdentityAndVariant(t *testing.T) {
	root := t.TempDir()
	writeTestRemoteHelperManifest(t, root, "standard")

	manifest, exists, err := ReadRemoteHelperManifest(root, "1.2.3", strings.Repeat("a", 40))
	if err != nil {
		t.Fatalf("ReadRemoteHelperManifest: %v", err)
	}
	if !exists || manifest.Variant != RemoteHelperVariantStandard || len(manifest.Helpers) != 4 {
		t.Fatalf("manifest = %#v, exists=%v", manifest, exists)
	}
}

func TestReadRemoteHelperManifestRejectsIdentityAndUnsafeAsset(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RemoteHelperManifest)
	}{
		{name: "version", mutate: func(m *RemoteHelperManifest) { m.Version = "9.9.9" }},
		{name: "commit", mutate: func(m *RemoteHelperManifest) { m.Commit = strings.Repeat("b", 40) }},
		{name: "path traversal", mutate: func(m *RemoteHelperManifest) { m.Helpers[0].Asset = "../agentctl" }},
		{name: "duplicate platform", mutate: func(m *RemoteHelperManifest) { m.Helpers[1].Platform = m.Helpers[0].Platform }},
		{name: "invalid digest", mutate: func(m *RemoteHelperManifest) { m.Helpers[0].SHA256 = "not-a-digest" }},
		{name: "invalid variant", mutate: func(m *RemoteHelperManifest) { m.Variant = "partial" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			manifest := testRemoteHelperManifest("standard")
			tt.mutate(&manifest)
			data, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, RemoteHelperManifestName), data, 0o644); err != nil {
				t.Fatal(err)
			}
			if _, _, err := ReadRemoteHelperManifest(root, "1.2.3", strings.Repeat("a", 40)); err == nil {
				t.Fatal("expected invalid manifest error")
			}
		})
	}
}

func TestReadRemoteHelperManifestMissingIsLegacyBundle(t *testing.T) {
	manifest, exists, err := ReadRemoteHelperManifest(t.TempDir(), "1.2.3", strings.Repeat("a", 40))
	if err != nil || exists || manifest != nil {
		t.Fatalf("ReadRemoteHelperManifest = (%#v, %v, %v), want (nil, false, nil)", manifest, exists, err)
	}
}

func writeTestRemoteHelperManifest(t *testing.T, root, variant string) {
	t.Helper()
	data, err := json.Marshal(testRemoteHelperManifest(variant))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, RemoteHelperManifestName), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func testRemoteHelperManifest(variant string) RemoteHelperManifest {
	return RemoteHelperManifest{
		SchemaVersion: RemoteHelperManifestSchemaVersion,
		Version:       "1.2.3",
		Commit:        strings.Repeat("a", 40),
		Variant:       variant,
		Helpers: []RemoteHelperRecord{
			{Platform: "linux/amd64", Asset: "agentctl-linux-amd64.gz", SHA256: strings.Repeat("a", 64), SizeBytes: 10},
			{Platform: "linux/arm64", Asset: "agentctl-linux-arm64.gz", SHA256: strings.Repeat("b", 64), SizeBytes: 10},
			{Platform: "darwin/amd64", Asset: "agentctl-darwin-amd64.gz", SHA256: strings.Repeat("c", 64), SizeBytes: 10},
			{Platform: "darwin/arm64", Asset: "agentctl-darwin-arm64.gz", SHA256: strings.Repeat("d", 64), SizeBytes: 10},
		},
	}
}
