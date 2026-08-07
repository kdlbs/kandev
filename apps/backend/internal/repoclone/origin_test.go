package repoclone

import "testing"

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
