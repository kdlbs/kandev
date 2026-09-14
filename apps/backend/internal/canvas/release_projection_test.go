package canvas

import (
	"encoding/json"
	"testing"
	"time"

	plugininstances "github.com/kandev/kandev/internal/plugins/instances"
	"github.com/kandev/kandev/internal/plugins/manifest"
)

func TestReleaseMetadataIncludesPortableManifestSeed(t *testing.T) {
	manifestJSON, err := json.Marshal(manifest.Manifest{
		ID: "canvas-one", Version: "1.0.0", DisplayName: "Canvas One", Description: "A canvas",
		Author: "Author", MinKandevVersion: "0.94.0", RepoURL: "https://example.test/canvas",
		Distribution: &manifest.Distribution{License: "MIT", SourceMode: manifest.SourceModeStatic},
	})
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	seed := releaseMetadata(plugininstances.Release{ID: "release-1", ManifestJSON: manifestJSON, CreatedAt: time.Now()}, ScopeWorkspace, nil)
	if seed.PackageID != "canvas-one" || seed.Version != "1.0.0" || seed.Author != "Author" || seed.License != "MIT" || seed.SourceMode != manifest.SourceModeStatic {
		t.Fatalf("release metadata seed = %+v", seed)
	}
}
