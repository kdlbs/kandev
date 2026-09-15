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
