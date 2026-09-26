package githubauth

import "testing"

func TestCanonicalCredentialPath(t *testing.T) {
	for name, testCase := range map[string]struct {
		raw  string
		want string
	}{
		"clone URL path":     {raw: "/acme/widgets.git", want: "/acme/widgets"},
		"shim path":          {raw: "/acme/widgets", want: "/acme/widgets"},
		"trailing slash":     {raw: "/acme/widgets/", want: "/acme/widgets"},
		"suffix then slash":  {raw: "/acme/widgets.git/", want: "/acme/widgets"},
		"surrounding spaces": {raw: "  /acme/widgets.git  ", want: "/acme/widgets"},
		"nested namespace":   {raw: "/scm/ENG/widgets.git", want: "/scm/ENG/widgets"},
		"case preserved":     {raw: "/Acme/Widgets.git", want: "/Acme/Widgets"},
		"root":               {raw: "/", want: "/"},
		"bare suffix":        {raw: "/.git", want: "/.git"},
		"empty":              {raw: "", want: ""},
		// GitHub forbids a repository named ".git"; reducing it keeps the
		// function idempotent instead of leaving a trailing slash behind.
		"repository named .git": {raw: "/acme/.git", want: "/acme"},
		// ".github" is a real repository name and must survive intact.
		"dot-prefixed repository": {raw: "/acme/.github", want: "/acme/.github"},
	} {
		t.Run(name, func(t *testing.T) {
			got := CanonicalCredentialPath(testCase.raw)
			if got != testCase.want {
				t.Fatalf("CanonicalCredentialPath(%q) = %q, want %q", testCase.raw, got, testCase.want)
			}
			if again := CanonicalCredentialPath(got); again != got {
				t.Fatalf("CanonicalCredentialPath(%q) is not idempotent: %q then %q", testCase.raw, got, again)
			}
		})
	}
}
