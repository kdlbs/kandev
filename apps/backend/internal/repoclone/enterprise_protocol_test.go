package repoclone

import "testing"

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-004.3
func TestEnterpriseCloneProtocol(t *testing.T) {
	for _, tc := range []struct {
		host, protocol, want string
		invalid              bool
	}{
		{"git.company.example:8443", ProtocolHTTPS, "https://git.company.example:8443/acme/repo.git", false},
		{"https://GIT.COMPANY.EXAMPLE:443/", ProtocolHTTPS, "https://git.company.example/acme/repo.git", false},
		{"git.company.example:8443", ProtocolSSH, "git@git.company.example:acme/repo.git", false},
		{"acme.ghe.com", ProtocolSSH, "git@acme.ghe.com:acme/repo.git", false},
		{"http://git.company.example", ProtocolHTTPS, "", true},
		{"https://git.company.example?token=ignored", ProtocolHTTPS, "", true},
		{"https://git.company.example#ignored", ProtocolHTTPS, "", true},
	} {
		t.Run(tc.host+tc.protocol, func(t *testing.T) {
			got, err := CloneURLWithHost("github", tc.host, "acme", "repo", tc.protocol)
			if tc.invalid {
				if err == nil {
					t.Fatal("insecure GitHub host accepted")
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("CloneURLWithHost = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}
