package gitlab

import "testing"

func TestGLabClientSatisfiesProviderActionContract(t *testing.T) {
	var _ Client = (*GLabClient)(nil)
}

func TestParseGlabToken_ExtractsValue(t *testing.T) {
	output := `Logged in to gitlab.com as alice (oauth_token)
✓ Token: glpat-AAAAA-BBBBB
✓ Token scopes: api`
	got := parseGlabToken(output)
	if got != "glpat-AAAAA-BBBBB" {
		t.Errorf("got %q, want glpat-AAAAA-BBBBB", got)
	}
}

func TestParseGlabToken_LowercaseLabel(t *testing.T) {
	got := parseGlabToken("token: glpat-xyz")
	if got != "glpat-xyz" {
		t.Errorf("got %q, want glpat-xyz", got)
	}
}

// glab >= 1.x prints "Token found:" rather than a bare "Token:" label.
func TestParseGlabToken_TokenFoundLabel(t *testing.T) {
	output := `gitlab.example.com
  ✓ Logged in to gitlab.example.com as example-user (/home/example/.config/glab-cli/config.yml)
  ✓ REST API Endpoint: https://gitlab.example.com/api/v4/
  ✓ Token found: glpat-AAAAA-BBBBB`
	got := parseGlabToken(output)
	if got != "glpat-AAAAA-BBBBB" {
		t.Errorf("got %q, want glpat-AAAAA-BBBBB", got)
	}
}

func TestParseGlabToken_NoToken(t *testing.T) {
	got := parseGlabToken("Token: <no token>")
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestParseGlabToken_NoTokenFound(t *testing.T) {
	if got := parseGlabToken("✓ Token found: <no token>"); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestParseGlabToken_Empty(t *testing.T) {
	if got := parseGlabToken(""); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestStripScheme(t *testing.T) {
	cases := map[string]string{
		"https://gitlab.com":          "gitlab.com",
		"http://gitlab.acme.corp":     "gitlab.acme.corp",
		"https://gitlab.com/":         "gitlab.com",
		"gitlab.example.com":          "gitlab.example.com",
		"https://gitlab.example.com/": "gitlab.example.com",
	}
	for in, want := range cases {
		if got := stripScheme(in); got != want {
			t.Errorf("stripScheme(%q) = %q, want %q", in, got, want)
		}
	}
}
