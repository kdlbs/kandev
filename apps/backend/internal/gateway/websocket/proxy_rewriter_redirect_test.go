package websocket

import "testing"

func TestRewriteAbsolutePathRejectsNetworkRelative(t *testing.T) {
	for _, target := range []string{`//external.example/path`, `/\external.example/path`} {
		for _, capability := range []string{"", "secret-capability"} {
			t.Run(target+capability, func(t *testing.T) {
				if got := rewriteAbsolutePath(target, proxyPrefix, capability); got != target {
					t.Fatalf("rewriteAbsolutePath(%q) = %q, want unchanged target", target, got)
				}
			})
		}
	}
}

func TestRewriteURLReferenceNetworkRelativeRedirects(t *testing.T) {
	cases := []struct {
		name   string
		target string
	}{
		{"slashes", `//external.example/path`},
		{"slash backslash", `/\external.example/path`},
		{"backslash slash", `\/external.example/path`},
		{"backslashes", `\\external.example/path`},
		{"leading whitespace", " \t/\\external.example/path"},
		{"embedded controls", "/\n\t\\external.example/path"},
	}
	for _, tc := range cases {
		for _, capability := range []string{"", "secret-capability"} {
			t.Run(tc.name+capability, func(t *testing.T) {
				if got := rewriteURLReference(tc.target, proxyPrefix, capability); got != tc.target {
					t.Fatalf("rewriteURLReference(%q) = %q, want unchanged target", tc.target, got)
				}
			})
		}
	}
}

func TestRewriteURLReferenceLocalRedirects(t *testing.T) {
	for _, target := range []string{"/", "/next?query=1#section"} {
		t.Run(target, func(t *testing.T) {
			if got := rewriteURLReference(target, proxyPrefix, ""); got != proxyPrefix+target {
				t.Fatalf("rewriteURLReference(%q) = %q, want %q", target, got, proxyPrefix+target)
			}
		})
	}
}
