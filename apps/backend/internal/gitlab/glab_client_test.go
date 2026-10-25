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

// glab 1.114 prints "Token found in configuration file (plaintext):" —
// neither of the two prior exact labels ("Token:", "Token found:") is a
// substring of this line, which is what silently broke token extraction
// (and surfaced as "GitLab connection unavailable" in the UI) even though
// `glab auth status` reported the user authenticated.
func TestParseGlabToken_TokenFoundInConfigFileLabel(t *testing.T) {
	output := `gitlab.example.com
  ✓ Logged in to gitlab.example.com as example-user (/home/example/.config/glab-cli/config.yml)
  ✓ REST API Endpoint: https://gitlab.example.com/api/v4/
  ✓ Token found in configuration file (plaintext): glpat-AAAAA-BBBBB
  ! To store this token more securely, run glab auth login --hostname gitlab.example.com to move it into the operating system keyring.`
	got := parseGlabToken(output)
	if got != "glpat-AAAAA-BBBBB" {
		t.Errorf("got %q, want glpat-AAAAA-BBBBB", got)
	}
}

// A future relabel this pattern was NOT written to anticipate verbatim —
// only structurally ("mentions token, ends in a colon then a value") — so a
// wording nobody has shipped yet should still parse.
func TestParseGlabToken_HypotheticalFutureLabel(t *testing.T) {
	got := parseGlabToken("✓ Token (from OS keyring): glpat-future-wording")
	if got != "glpat-future-wording" {
		t.Errorf("got %q, want glpat-future-wording", got)
	}
}

// Without -t (or when the CLI redacts on display), glab prints the token as
// a run of asterisks rather than omitting the line. That must not be mistaken
// for a real token.
func TestParseGlabToken_MaskedTokenIsNotAToken(t *testing.T) {
	got := parseGlabToken("✓ Token found in configuration file (plaintext): **************************")
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestParseGlabToken_MaskedTokenDoesNotFallThroughToScopes(t *testing.T) {
	output := `gitlab.example.com
  ✓ Logged in to gitlab.example.com as example-user (/home/example/.config/glab-cli/config.yml)
  ✓ Token found in configuration file (plaintext): **************************
  ✓ Token scopes: api`
	if got := parseGlabToken(output); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestParseGlabToken_DiagnosticLineIsNotAToken(t *testing.T) {
	if got := parseGlabToken("error: token exchange failed: unauthorized"); got != "" {
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
