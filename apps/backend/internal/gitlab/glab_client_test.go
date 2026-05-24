package gitlab

import "testing"

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

func TestParseGlabToken_NoToken(t *testing.T) {
	got := parseGlabToken("Token: <no token>")
	if got != "" {
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
