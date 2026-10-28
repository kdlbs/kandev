package config

import (
	"testing"

	"github.com/kandev/kandev/internal/githubauth"
)

func TestEffectiveBrokerURLEnv(t *testing.T) {
	const upstream = "http://backend:8080/api/v1/git/credentials/resolve"
	env := map[string]string{githubauth.CredentialBrokerURLEnv: upstream, "K": "v"}
	cfg := &InstanceConfig{Port: 9001, BrokerUpstreamURL: upstream}
	got := cfg.EffectiveBrokerURLEnv(env)
	if got[githubauth.CredentialBrokerURLEnv] != "http://127.0.0.1:9001/git-credentials/resolve" || got["K"] != "v" {
		t.Fatalf("relay env = %#v", got)
	}
	if env[githubauth.CredentialBrokerURLEnv] != upstream {
		t.Fatal("input env modified")
	}
	for name, c := range map[string]*InstanceConfig{
		"nil": nil, "no upstream": {Port: 9001}, "no port": {BrokerUpstreamURL: upstream},
	} {
		if out := c.EffectiveBrokerURLEnv(env); out[githubauth.CredentialBrokerURLEnv] != upstream {
			t.Errorf("%s rewrote env: %#v", name, out)
		}
	}
	noBroker := map[string]string{"K": "v"}
	if out := cfg.EffectiveBrokerURLEnv(noBroker); out[githubauth.CredentialBrokerURLEnv] != "" {
		t.Errorf("env without broker URL gained one: %#v", out)
	}
}
