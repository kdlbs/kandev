package repoclone

import "testing"

func TestCanonicalHTTPSCloneURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "scp style", raw: "git@github.com:acme/widgets.git", want: "https://github.com/acme/widgets.git"},
		{name: "scp style uppercase host", raw: "git@GitHub.com:acme/widgets.git", want: "https://github.com/acme/widgets.git"},
		{name: "scp style leading slash", raw: "git@github.com:/acme/widgets.git", want: "https://github.com/acme/widgets.git"},
		{name: "scp style nested namespace", raw: "git@gitlab.example:group/sub/widgets.git", want: "https://gitlab.example/group/sub/widgets.git"},
		{name: "ssh scheme", raw: "ssh://git@github.com/acme/widgets.git", want: "https://github.com/acme/widgets.git"},
		{name: "ssh scheme with port", raw: "ssh://git@gitlab.example:2222/group/widgets.git", want: "https://gitlab.example/group/widgets.git"},
		{name: "ssh scheme drops password", raw: "ssh://git:secret@github.com/acme/widgets.git", want: "https://github.com/acme/widgets.git"},
		{name: "https passes through untouched", raw: "https://github.com/acme/widgets.git"},
		{name: "file remote", raw: "file:///tmp/widgets.git"},
		{name: "local path", raw: "/home/user/widgets"},
		{name: "windows path", raw: `C:\repos\widgets`},
		{name: "empty", raw: ""},
		{name: "ssh scheme without path", raw: "ssh://git@github.com/"},
		{name: "scp style without path", raw: "git@github.com:"},
		{name: "scp style without host", raw: "git@:acme/widgets.git"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := CanonicalHTTPSCloneURL(test.raw); got != test.want {
				t.Fatalf("CanonicalHTTPSCloneURL(%q) = %q, want %q", test.raw, got, test.want)
			}
		})
	}
}

func TestValidateHTTPSCloneOrigin(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		cloneURL     string
		providerHost string
		wantErr      bool
	}{
		{
			name:         "same origin with provider context path",
			cloneURL:     "https://bitbucket.example.test/context/scm/TEAM/repo.git",
			providerHost: "https://BITBUCKET.example.test/context",
		},
		{
			name:         "bare provider hostname",
			cloneURL:     "https://bitbucket.org/team/repo.git",
			providerHost: "bitbucket.org",
		},
		{
			name:         "different host",
			cloneURL:     "https://attacker.example/repo.git",
			providerHost: "https://bitbucket.example.test",
			wantErr:      true,
		},
		{
			name:         "different port",
			cloneURL:     "https://bitbucket.example.test:8443/repo.git",
			providerHost: "https://bitbucket.example.test",
			wantErr:      true,
		},
		{
			name:         "userinfo",
			cloneURL:     "https://token@bitbucket.example.test/repo.git",
			providerHost: "https://bitbucket.example.test",
			wantErr:      true,
		},
		{
			name:         "non HTTPS",
			cloneURL:     "http://bitbucket.example.test/repo.git",
			providerHost: "https://bitbucket.example.test",
			wantErr:      true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateHTTPSCloneOrigin(test.cloneURL, test.providerHost)
			if (err != nil) != test.wantErr {
				t.Fatalf("ValidateHTTPSCloneOrigin() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}
