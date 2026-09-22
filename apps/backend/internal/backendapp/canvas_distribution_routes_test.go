package backendapp

import (
	"errors"
	"net/http"
	"testing"

	canvasservice "github.com/kandev/kandev/internal/canvas"
)

func TestCanvasDistributionErrorStatusUsesInternalErrorForUnexpectedFailures(t *testing.T) {
	status, code := canvasDistributionErrorStatus(errors.New("database unavailable"))
	if status != http.StatusInternalServerError || code != "internal_error" {
		t.Fatalf("status = %d %q, want 500 internal_error", status, code)
	}
}

func TestCanvasDistributionErrorStatusMapsIncompatibleInstall(t *testing.T) {
	status, code := canvasDistributionErrorStatus(canvasservice.ErrInstallIncompatible)
	if status != http.StatusConflict || code != "incompatible_install" {
		t.Fatalf("status = %d %q, want 409 incompatible_install", status, code)
	}
}

func TestCanvasReleaseResponseIncludesSafeDistributionFields(t *testing.T) {
	response := releaseResponseFromMetadata(&canvasservice.ReleaseMetadata{
		ID: "release-1", PackageID: "canvas-example", Version: "1.1.0", DisplayName: "Example",
		Description: "Example canvas", Author: "Author", License: "MIT", SourceMode: "static",
		MinKandevVersion: "0.95.0", RepoURL: "https://example.test/canvas",
	})
	if response.PackageID != "canvas-example" || response.Version != "1.1.0" || response.DisplayName != "Example" || response.Description != "Example canvas" || response.Author != "Author" || response.License != "MIT" || response.SourceMode != "static" || response.MinKandevVersion != "0.95.0" || response.RepoURL != "https://example.test/canvas" {
		t.Fatalf("distribution projection = %+v", response)
	}
}
