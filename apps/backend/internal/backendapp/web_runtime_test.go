package backendapp

import (
	"net/http/httptest"
	"slices"
	"testing"

	lspinstaller "github.com/kandev/kandev/internal/lsp/installer"
)

func TestWebRuntimeConfigAdvertisesLSPAutoInstallCapabilities(t *testing.T) {
	request := httptest.NewRequest("GET", "/settings/editors", nil)
	got := webRuntimeConfig(false, request).LSPAutoInstallSupportedLanguages
	want := lspinstaller.AutoInstallLanguages()
	if !slices.Equal(got, want) {
		t.Fatalf("LSP auto-install capabilities = %v, want %v", got, want)
	}
}
