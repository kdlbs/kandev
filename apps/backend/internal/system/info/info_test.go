package info

import (
	"encoding/json"
	"net/http/httptest"
	"regexp"
	"runtime"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestInfo_ReturnsConfiguredAndRuntimeFields(t *testing.T) {
	s := NewService("v1.2.3", "abc123", "2026-05-18T00:00:00Z")
	got := s.Info()
	if got.Version != "v1.2.3" {
		t.Errorf("Version = %q", got.Version)
	}
	if got.Commit != "abc123" {
		t.Errorf("Commit = %q", got.Commit)
	}
	if got.BuildTime != "2026-05-18T00:00:00Z" {
		t.Errorf("BuildTime = %q", got.BuildTime)
	}
	if got.GoVersion != runtime.Version() {
		t.Errorf("GoVersion = %q, want %q", got.GoVersion, runtime.Version())
	}
	if got.OS != runtime.GOOS {
		t.Errorf("OS = %q", got.OS)
	}
	if got.Arch != runtime.GOARCH {
		t.Errorf("Arch = %q", got.Arch)
	}
	if got.BootID == "" {
		t.Fatal("BootID empty")
	}
	if got.StartedAt == "" {
		t.Fatal("StartedAt empty")
	}
	if again := s.Info(); again.BootID != got.BootID {
		t.Fatalf("BootID changed within process: %q != %q", again.BootID, got.BootID)
	}
}

func TestHandler_RendersJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := NewService("v1.2.3", "abc", "now")
	r := gin.New()
	r.GET("/info", Handler(s))

	req := httptest.NewRequest("GET", "/info", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if resp.Version != "v1.2.3" {
		t.Errorf("Version = %q", resp.Version)
	}
	if resp.BootID == "" {
		t.Fatal("BootID empty")
	}
	if resp.StartedAt == "" {
		t.Fatal("StartedAt empty")
	}
}

func TestNewBootID_ReturnsHexEncodedBytes(t *testing.T) {
	got := newBootID()
	if matched := regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(got); !matched {
		t.Fatalf("BootID = %q, want 32 lowercase hex chars", got)
	}
}
