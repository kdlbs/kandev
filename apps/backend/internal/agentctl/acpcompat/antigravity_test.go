package acpcompat

import (
	"errors"
	"strings"
	"testing"
)

const validConsentURL = "https://accounts.google.com/o/oauth2/v2/auth?client_id=client&state=state&code_challenge=challenge&redirect_uri=http%3A%2F%2F127.0.0.1%3A5555%2Fcallback"

func TestAntigravityClientInfo(t *testing.T) {
	name, version := ClientInfoForAgent(AntigravityAgentID)
	if name != "zed" || version != "0.0.0" {
		t.Fatalf("client info = %q/%q", name, version)
	}
	if name, _ := ClientInfoForAgent("claude-acp"); name != "" {
		t.Fatalf("other agent got override %q", name)
	}
	if !IsHumanInputPermission(AntigravityAgentID, "interaction_123") {
		t.Error("interaction_ prefix was not recognized")
	}
	if IsHumanInputPermission("other-acp", "interaction_123") {
		t.Error("other agent inherited the Antigravity prefix")
	}
	if IsHumanInputPermission(AntigravityAgentID, "bash-1") {
		t.Error("ordinary tool call was classified as human input")
	}
}

func TestAntigravityConsentURL(t *testing.T) {
	raw, ok := ParseConsentURL("Open this URL: " + validConsentURL)
	if !ok || raw != validConsentURL {
		t.Fatalf("ParseConsentURL = %q, %v", raw, ok)
	}
	if _, err := ValidateConsentURL(validConsentURL); err != nil {
		t.Fatalf("valid URL rejected: %v", err)
	}
	cases := []string{
		"https://accounts.google.com/o/oauth2/v2/auth?client_id=x&state=y&redirect_uri=http%3A%2F%2F127.0.0.1%3A5555%2Fcb",
		"http://accounts.google.com/o/oauth2/v2/auth?client_id=x&state=y&redirect_uri=http%3A%2F%2F127.0.0.1%3A5555%2Fcb",
		"https://evil.example/o/oauth2/v2/auth?client_id=x&state=y&redirect_uri=http%3A%2F%2F127.0.0.1%3A5555%2Fcb",
		"https://accounts.google.com/o/oauth2/v2/auth?client_id=x&state=y&redirect_uri=https%3A%2F%2Fevil.example%2Fcb",
		"https://accounts.google.com/o/oauth2/v2/auth?client_id=x&state=y&redirect_uri=http%3A%2F%2F10.0.0.1%3A5555%2Fcb",
		"https://user@accounts.google.com/o/oauth2/v2/auth?client_id=x&state=y&redirect_uri=http%3A%2F%2F127.0.0.1%3A5555%2Fcb",
	}
	for _, raw := range cases {
		if _, err := ValidateConsentURL(raw); err == nil {
			t.Errorf("unsafe URL accepted: %s", raw)
		}
	}
}

func TestAntigravityEnvironmentAndBrowser(t *testing.T) {
	names := AntigravityCredentialEnvNames()
	for _, want := range []string{"GEMINI_API_KEY", "GOOGLE_API_KEY", "GOOGLE_APPLICATION_CREDENTIALS"} {
		found := false
		for _, name := range names {
			if name == want {
				found = true
			}
		}
		if !found {
			t.Errorf("credential env %s missing", want)
		}
	}
	if err := ValidateLocalAntigravityEnvironment(map[string]string{"GEMINI_HOME": "/tmp/other"}, "/home/user"); err == nil {
		t.Error("non-empty GEMINI_HOME was accepted")
	}
	if err := ValidateLocalAntigravityEnvironment(map[string]string{"HOME": "/tmp/other"}, "/home/user"); err == nil {
		t.Error("conflicting HOME was accepted")
	}
	if err := ValidateLocalAntigravityEnvironment(map[string]string{"HOME": "/home/user"}, "/home/user"); err != nil {
		t.Errorf("matching HOME rejected: %v", err)
	}
	value := BrowserEnvValue(`C:\Program Files\Kandev\agentctl.exe`)
	if !strings.Contains(value, AntigravityBrowserHelperCommand) || !strings.HasPrefix(value, `"C:\\Program Files`) {
		t.Errorf("BrowserEnvValue = %q", value)
	}
	plain := BrowserEnvValue(`C:\Users\alice\.kandev\agentctl.exe`)
	if !strings.HasPrefix(plain, `"C:\\Users\\alice\\.kandev\\agentctl.exe"`) {
		t.Errorf("plain Windows path was not Python-quoted: %q", plain)
	}
	if !errors.Is(&AuthRequiredError{Message: "sign in"}, ErrAuthRequired) {
		t.Error("AuthRequiredError does not unwrap ErrAuthRequired")
	}
}
