package githubhost

import "testing"

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-001.2
func TestHostEndpoints(t *testing.T) {
	for _, tc := range []struct{ input, authority, rest, graphql, token string }{
		{"HTTPS://GitHub.COM:443/", "github.com", "https://api.github.com", "https://api.github.com/graphql", "GH_TOKEN"},
		{"tenant.ghe.com", "tenant.ghe.com", "https://api.tenant.ghe.com", "https://api.tenant.ghe.com/graphql", "GH_TOKEN"},
		{"git.example.com:8443", "git.example.com:8443", "https://git.example.com:8443/api/v3", "https://git.example.com:8443/api/graphql", "GH_ENTERPRISE_TOKEN"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			h, err := Parse(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if h.String() != tc.authority || h.WebURL() != "https://"+tc.authority || h.RESTURL() != tc.rest || h.GraphQLURL() != tc.graphql || h.TokenVariable() != tc.token {
				t.Fatalf("incorrect host endpoints: %#v %s %s %s", h, h.RESTURL(), h.GraphQLURL(), h.TokenVariable())
			}
		})
	}
}
func TestHostInvalid(t *testing.T) {
	for _, input := range []string{"", "http://git.example.com", "https://user@git.example.com", "https://git.example.com/repo", "https://git.example.com?x", "https://git.example.com#", "127.0.0.1", "[::1]", "git.example.com:0", "git.example.com:65536", "git.example.com:abc", "git.example.com:", " git.example.com", "git.example.com\n", "api.github.com", "github.com:8443", "ghe.com", "api.tenant.ghe.com", "tenant.ghe.com:8443", "bad..example.com", "-bad.example.com"} {
		t.Run(input, func(t *testing.T) {
			if _, err := Parse(input); err == nil {
				t.Fatalf("accepted invalid host %q", input)
			}
		})
	}
}
