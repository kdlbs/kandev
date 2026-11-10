package antigravityruntime

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveAntigravityBundle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses POSIX executable bits")
	}
	t.Run("manual complete install", func(t *testing.T) {
		dir := t.TempDir()
		server := filepath.Join(dir, "agy_acp_server.par")
		harness := filepath.Join(dir, "localharness_external")
		writeExecutable(t, server, "server")
		writeExecutable(t, harness, "harness")
		bundle, err := Resolve(context.Background(), ResolveOptions{
			GOOS:    "linux",
			GOARCH:  "amd64",
			HomeDir: t.TempDir(),
			LookPath: func(string) (string, error) {
				return server, nil
			},
		})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if bundle.Managed || bundle.ServerPath != server || bundle.HarnessPath != harness {
			t.Fatalf("bundle = %+v", bundle)
		}
	})

	t.Run("invalid managed destination fails closed", func(t *testing.T) {
		home := t.TempDir()
		artifact, err := ArtifactFor("linux", "amd64")
		if err != nil {
			t.Fatal(err)
		}
		dest := ManagedDestination(home, artifact)
		if err := os.MkdirAll(dest, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dest, ManifestName), []byte("{bad"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err = Resolve(context.Background(), ResolveOptions{
			GOOS: "linux", GOARCH: "amd64", HomeDir: home,
			LookPath: func(string) (string, error) { return "", errors.New("must not fall back") },
		})
		if !errors.Is(err, ErrInvalidManagedBundle) {
			t.Fatalf("error = %v, want ErrInvalidManagedBundle", err)
		}
	})

	t.Run("invalid harness override is actionable", func(t *testing.T) {
		dir := t.TempDir()
		server := filepath.Join(dir, "agy_acp_server.par")
		writeExecutable(t, server, "server")
		_, err := Resolve(context.Background(), ResolveOptions{
			GOOS: "linux", GOARCH: "amd64", HomeDir: t.TempDir(),
			Env:      map[string]string{"ANTIGRAVITY_HARNESS_PATH": filepath.Join(dir, "missing")},
			LookPath: func(string) (string, error) { return server, nil },
		})
		if !errors.Is(err, ErrInvalidHarnessOverride) {
			t.Fatalf("error = %v, want ErrInvalidHarnessOverride", err)
		}
	})

	t.Run("no runtime is unavailable", func(t *testing.T) {
		_, err := Resolve(context.Background(), ResolveOptions{
			GOOS: "linux", GOARCH: "amd64", HomeDir: t.TempDir(),
			LookPath: func(string) (string, error) { return "", errors.New("missing") },
		})
		if !errors.Is(err, ErrUnavailable) {
			t.Fatalf("error = %v, want ErrUnavailable", err)
		}
	})
}

func TestInstallAntigravityBundle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses POSIX executable bits")
	}
	t.Run("installs and is idempotent", func(t *testing.T) {
		archive := buildArchive(t, map[string]string{
			"agy_acp_server.par":    "server",
			"localharness_external": "harness",
		})
		digest := sha256.Sum256(archive)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(archive)
		}))
		defer server.Close()
		artifact := Artifact{
			Version: Version, OS: "linux", Arch: "amd64", URL: server.URL,
			SHA256: hex.EncodeToString(digest[:]), CompressedLimit: 1 << 20,
			ExpandedLimit: 1 << 20, ServerName: "agy_acp_server.par",
			HarnessNames: []string{"localharness_external"},
		}
		home := t.TempDir()
		bundle, err := Install(context.Background(), InstallOptions{
			GOOS: "linux", GOARCH: "amd64", HomeDir: home, Artifact: &artifact,
		})
		if err != nil {
			t.Fatalf("Install: %v", err)
		}
		if !bundle.Managed {
			t.Fatal("bundle.Managed = false")
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(bundle.ServerPath), ManifestName)); err != nil {
			t.Fatalf("manifest missing: %v", err)
		}
		again, err := Install(context.Background(), InstallOptions{
			GOOS: "linux", GOARCH: "amd64", HomeDir: home, Artifact: &artifact,
		})
		if err != nil || again.ServerPath != bundle.ServerPath {
			t.Fatalf("idempotent install = %+v, %v", again, err)
		}
	})

	t.Run("digest mismatch preserves previous state", func(t *testing.T) {
		archive := buildArchive(t, map[string]string{
			"agy_acp_server.par":    "server",
			"localharness_external": "harness",
		})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(archive)
		}))
		defer server.Close()
		artifact := Artifact{
			Version: Version, OS: "linux", Arch: "amd64", URL: server.URL,
			SHA256: "deadbeef", CompressedLimit: 1 << 20, ExpandedLimit: 1 << 20,
			ServerName: "agy_acp_server.par", HarnessNames: []string{"localharness_external"},
		}
		home := t.TempDir()
		sentinel := filepath.Join(ManagedRoot(home), "existing-bundle")
		if err := os.MkdirAll(sentinel, 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := Install(context.Background(), InstallOptions{
			GOOS: "linux", GOARCH: "amd64", HomeDir: home, Artifact: &artifact,
		})
		if !errors.Is(err, ErrDigestMismatch) {
			t.Fatalf("error = %v, want ErrDigestMismatch", err)
		}
		if _, statErr := os.Stat(sentinel); statErr != nil {
			t.Fatalf("existing bundle was disturbed: %v", statErr)
		}
	})

	t.Run("unsafe archive entry is rejected", func(t *testing.T) {
		archive := buildArchive(t, map[string]string{
			"agy_acp_server.par":    "server",
			"localharness_external": "harness",
			"../escape":             "bad",
		})
		digest := sha256.Sum256(archive)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(archive)
		}))
		defer server.Close()
		artifact := Artifact{
			Version: Version, OS: "linux", Arch: "amd64", URL: server.URL,
			SHA256: hex.EncodeToString(digest[:]), CompressedLimit: 1 << 20,
			ExpandedLimit: 1 << 20, ServerName: "agy_acp_server.par",
			HarnessNames: []string{"localharness_external"},
		}
		_, err := Install(context.Background(), InstallOptions{
			GOOS: "linux", GOARCH: "amd64", HomeDir: t.TempDir(), Artifact: &artifact,
		})
		if !errors.Is(err, ErrUnsafeArchive) {
			t.Fatalf("error = %v, want ErrUnsafeArchive", err)
		}
	})

	t.Run("cancellation removes staging", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		artifact := Artifact{
			Version: Version, OS: "linux", Arch: "amd64", URL: "https://example.invalid",
			SHA256: "x", CompressedLimit: 1, ExpandedLimit: 1,
			ServerName: "agy_acp_server.par", HarnessNames: []string{"localharness_external"},
		}
		_, err := Install(ctx, InstallOptions{
			GOOS: "linux", GOARCH: "amd64", HomeDir: t.TempDir(), Artifact: &artifact,
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	})
}

func TestInstallAntigravityCancellationDoesNotPublish(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses POSIX executable bits")
	}
	archive := buildArchive(t, map[string]string{
		"agy_acp_server.par":    "server",
		"localharness_external": "harness",
	})
	digest := sha256.Sum256(archive)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	artifact := Artifact{
		Version: Version, OS: "linux", Arch: "amd64", URL: server.URL,
		SHA256: hex.EncodeToString(digest[:]), CompressedLimit: 1 << 20,
		ExpandedLimit: 1 << 20, ServerName: "agy_acp_server.par",
		HarnessNames: []string{"localharness_external"},
	}
	cases := []struct {
		name  string
		setup func(context.CancelFunc) InstallOptions
	}{
		{"after download", func(cancel context.CancelFunc) InstallOptions {
			return InstallOptions{afterDownload: cancel}
		}},
		{"during extraction", func(cancel context.CancelFunc) InstallOptions {
			return InstallOptions{duringExtract: cancel}
		}},
		{"before publication", func(cancel context.CancelFunc) InstallOptions {
			return InstallOptions{afterExtract: cancel}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			opts := tc.setup(cancel)
			opts.GOOS, opts.GOARCH, opts.HomeDir, opts.Artifact = "linux", "amd64", t.TempDir(), &artifact
			_, err := Install(ctx, opts)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context.Canceled", err)
			}
			if _, statErr := os.Stat(ManagedDestination(opts.HomeDir, artifact)); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("cancelled install published destination: %v", statErr)
			}
			entries, readErr := os.ReadDir(ManagedRoot(opts.HomeDir))
			if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
				t.Fatalf("read managed root: %v", readErr)
			}
			for _, entry := range entries {
				if len(entry.Name()) >= len(".staging-") && entry.Name()[:len(".staging-")] == ".staging-" {
					t.Fatalf("cancelled install left staging entry %s", entry.Name())
				}
			}
		})
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func buildArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestManifestJSON(t *testing.T) {
	raw, err := json.Marshal(Manifest{Version: Version})
	if err != nil || len(raw) == 0 {
		t.Fatalf("marshal manifest: %v", err)
	}
}
