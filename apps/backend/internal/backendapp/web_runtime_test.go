package backendapp

import (
	"net/http/httptest"
	"slices"
	"testing"

	lspinstaller "github.com/kandev/kandev/internal/lsp/installer"
)

func TestWebRuntimeConfigAdvertisesLSPAutoInstallPreferences(t *testing.T) {
	request := httptest.NewRequest("GET", "/settings/editors", nil)
	got := webRuntimeConfig(false, "", request).LSPAutoInstallPreferenceLanguages
	want := lspinstaller.AutoInstallPreferenceLanguages()
	if !slices.Equal(got, want) {
		t.Fatalf("LSP auto-install preferences = %v, want %v", got, want)
	}
}

func TestWebRuntimeConfigTrimsTitlePrefix(t *testing.T) {
	request := httptest.NewRequest("GET", "/", nil)
	if got := webRuntimeConfig(false, "  TEST  ", request).TitlePrefix; got != "TEST" {
		t.Fatalf("title prefix = %q, want %q", got, "TEST")
	}
}

func TestWebRuntimeConfigOmitsBlankTitlePrefix(t *testing.T) {
	request := httptest.NewRequest("GET", "/", nil)
	if got := webRuntimeConfig(false, "   ", request).TitlePrefix; got != "" {
		t.Fatalf("title prefix = %q, want empty", got)
	}
}
