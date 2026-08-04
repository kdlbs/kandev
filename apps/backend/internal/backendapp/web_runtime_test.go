package backendapp

import (
	"net/http/httptest"
	"slices"
	"testing"

	lspinstaller "github.com/kandev/kandev/internal/lsp/installer"
)

func TestWebRuntimeConfigAdvertisesLSPAutoInstallPreferences(t *testing.T) {
	request := httptest.NewRequest("GET", "/settings/editors", nil)
	got := webRuntimeConfig(false, request).LSPAutoInstallPreferenceLanguages
	want := lspinstaller.AutoInstallPreferenceLanguages()
	if !slices.Equal(got, want) {
		t.Fatalf("LSP auto-install preferences = %v, want %v", got, want)
	}
}
