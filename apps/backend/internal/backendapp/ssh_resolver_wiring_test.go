package backendapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
)

func TestSSHResolverUsesBuildIdentityForStandardManifest(t *testing.T) {
	const (
		version = "1.2.3"
		commit  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)
	payload := []byte("cached remote helper")
	digest := sha256.Sum256(payload)
	digestText := hex.EncodeToString(digest[:])
	platform := lifecycle.SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}
	bundleDir := t.TempDir()
	homeDir := t.TempDir()
	manifest := lifecycle.RemoteHelperManifest{
		SchemaVersion: lifecycle.RemoteHelperManifestSchemaVersion,
		Version:       version,
		Commit:        commit,
		Variant:       lifecycle.RemoteHelperVariantStandard,
		Helpers: []lifecycle.RemoteHelperRecord{
			{Platform: "linux/amd64", Asset: "agentctl-linux-amd64.gz", SHA256: digestText, SizeBytes: int64(len(payload))},
			{Platform: "linux/arm64", Asset: "agentctl-linux-arm64.gz", SHA256: digestText, SizeBytes: int64(len(payload))},
			{Platform: "darwin/amd64", Asset: "agentctl-darwin-amd64.gz", SHA256: digestText, SizeBytes: int64(len(payload))},
			{Platform: "darwin/arm64", Asset: "agentctl-darwin-arm64.gz", SHA256: digestText, SizeBytes: int64(len(payload))},
		},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal remote helper manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bundleDir, lifecycle.RemoteHelperManifestName), manifestBytes, 0o600); err != nil {
		t.Fatalf("write remote helper manifest: %v", err)
	}
	cachePath := filepath.Join(homeDir, "cache", "remote-helpers", version, "linux-amd64", digestText, "agentctl")
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o700); err != nil {
		t.Fatalf("create helper cache: %v", err)
	}
	if err := os.WriteFile(cachePath, payload, 0o700); err != nil {
		t.Fatalf("seed helper cache: %v", err)
	}
	t.Setenv("KANDEV_BUNDLE_DIR", bundleDir)

	resolver := newSSHAgentctlResolver(routeParams{
		version: version,
		commit:  commit,
		homeDir: homeDir,
		log:     newTestLogger(),
	})
	got, err := resolver.ResolveRemoteBinaryContext(context.Background(), platform, nil)
	if err != nil {
		t.Fatalf("resolve helper from standard bundle: %v", err)
	}
	if got != cachePath {
		t.Fatalf("resolved helper path = %q, want %q", got, cachePath)
	}
}
