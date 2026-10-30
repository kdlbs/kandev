package lifecycle

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
)

func TestSSHRemoteAgentEnvForwardsPiRemoteAuthKeys(t *testing.T) {
	env := map[string]string{}
	for _, method := range agents.NewPiACP().RemoteAuth().Methods {
		if method.Type == "env" {
			env[method.EnvVar] = "secret-for-" + method.EnvVar
		}
	}
	if len(env) == 0 {
		t.Fatal("Pi declares no environment-variable methods")
	}

	got := sshRemoteAgentEnv(&ExecutorCreateRequest{Env: env})
	for key, want := range env {
		if got[key] != want {
			t.Errorf("%s = %q, want the resolved secret forwarded to the SSH agent", key, got[key])
		}
	}
}
