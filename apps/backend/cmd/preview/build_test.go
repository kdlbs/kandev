package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreviewArtifactExists(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	missing := filepath.Join(tempDir, "missing.tar.gz")
	if exists, err := previewArtifactExists(missing); err != nil || exists {
		t.Fatalf("previewArtifactExists(missing) = (%v, %v), want (false, nil)", exists, err)
	}

	if exists, err := previewArtifactExists(tempDir); err == nil || exists {
		t.Fatalf("previewArtifactExists(directory) = (%v, %v), want (false, error)", exists, err)
	}

	artifact := filepath.Join(tempDir, "preview.tar.gz")
	if err := os.WriteFile(artifact, []byte("bundle"), 0o600); err != nil {
		t.Fatal(err)
	}
	if exists, err := previewArtifactExists(artifact); err != nil || !exists {
		t.Fatalf("previewArtifactExists(file) = (%v, %v), want (true, nil)", exists, err)
	}
}

func TestUntrustedBuildEnvRemovesCredentials(t *testing.T) {
	t.Parallel()

	buildEnv := untrustedBuildEnv([]string{
		"PATH=/usr/bin",
		"HOME=/tmp/kandev",
		"SPRITES_API_TOKEN=sprites-secret",
		"GH_TOKEN=github-secret",
		"GITHUB_TOKEN=github-actions-secret",
		"ACTIONS_ID_TOKEN_REQUEST_TOKEN=oidc-secret",
		"ACTIONS_RUNTIME_TOKEN=runtime-secret",
	})

	got := strings.Join(buildEnv, "\n")
	for _, credential := range []string{
		"SPRITES_API_TOKEN=", "GH_TOKEN=", "GITHUB_TOKEN=", "ACTIONS_ID_TOKEN_REQUEST_TOKEN=", "ACTIONS_RUNTIME_TOKEN=",
	} {
		if strings.Contains(got, credential) {
			t.Fatalf("untrustedBuildEnv() retained %q", credential)
		}
	}
	if got != "PATH=/usr/bin\nHOME=/tmp/kandev" {
		t.Fatalf("untrustedBuildEnv() = %q, want preserved non-credential environment", got)
	}
}

func TestViteIndexHasEntrypoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		html string
		want bool
	}{
		{
			name: "vite dist index",
			html: `<!doctype html><html><head><script type="module" crossorigin src="/assets/index-abc123.js"></script></head><body><div id="root"></div></body></html>`,
			want: true,
		},
		{
			name: "vite dist index with src before type and single quotes",
			html: `<!doctype html><html><head><script src='/assets/index-abc123.js' crossorigin type='module'></script></head><body><div id="root"></div></body></html>`,
			want: true,
		},
		{
			name: "vite dist index with spaced attribute assignment",
			html: `<!doctype html><html><head><script type = "module" crossorigin src = "/assets/index-abc123.js"></script></head><body><div id="root"></div></body></html>`,
			want: true,
		},
		{
			name: "fallback shell has no app entrypoint",
			html: `<!doctype html><html><head><title>Kandev</title></head><body><div id="root"></div></body></html>`,
			want: false,
		},
		{
			name: "source index still points at vite dev source",
			html: `<!doctype html><html><body><div id="root"></div><script type="module" src="/src/main.tsx"></script></body></html>`,
			want: false,
		},
		{
			name: "inline module script plus unrelated assets reference",
			html: `<!doctype html><html><head><link rel="stylesheet" href="/assets/index.css"></head><body><script type="module">console.log("x")</script></body></html>`,
			want: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := viteIndexHasEntrypoint(tc.html); got != tc.want {
				t.Fatalf("viteIndexHasEntrypoint() = %v, want %v", got, tc.want)
			}
		})
	}
}
