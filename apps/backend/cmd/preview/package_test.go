package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRunPackageRejectsMalformedFlags(t *testing.T) {
	if got := runPackage(context.Background(), []string{"--unknown"}); got != 2 {
		t.Fatalf("runPackage() = %d, want 2", got)
	}
}

func TestRunPackageReturnsErrorWhenArtifactDirectoryCannotBeCreated(t *testing.T) {
	parentFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parentFile, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	buildCalled := false
	got := runPackageWithArtifactBuilder(context.Background(), []string{"--artifact", filepath.Join(parentFile, "preview.tar.gz")}, func(context.Context, string, string, bool, bool) error {
		buildCalled = true
		return nil
	})
	if got != 1 {
		t.Fatalf("runPackageWithArtifactBuilder() = %d, want 1", got)
	}
	if buildCalled {
		t.Fatal("runPackageWithArtifactBuilder() called the artifact builder after directory setup failed")
	}
}

func TestRunPackageForwardsSkipWebBuildToArtifactBuilder(t *testing.T) {
	artifact := filepath.Join(t.TempDir(), "preview", "preview.tar.gz")
	var gotSkipWebBuild bool
	buildCalled := false

	got := runPackageWithArtifactBuilder(context.Background(), []string{"--artifact", artifact, "--skip-web-build"}, func(_ context.Context, binDir, gotArtifact string, skipWebInstall, skipWebBuild bool) error {
		buildCalled = true
		if binDir == "" {
			t.Fatal("artifact builder received an empty binary directory")
		}
		if gotArtifact != artifact {
			t.Fatalf("artifact builder artifact = %q, want %q", gotArtifact, artifact)
		}
		if skipWebInstall {
			t.Fatal("artifact builder skipWebInstall = true, want false")
		}
		gotSkipWebBuild = skipWebBuild
		return nil
	})
	if got != 0 {
		t.Fatalf("runPackageWithArtifactBuilder() = %d, want 0", got)
	}
	if !buildCalled {
		t.Fatal("runPackageWithArtifactBuilder() did not call the artifact builder")
	}
	if !gotSkipWebBuild {
		t.Fatal("artifact builder skipWebBuild = false, want true")
	}
}
