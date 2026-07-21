package installer

import (
	"context"
	"os"
	"os/exec"
	"strings"
)

func combinedOutput(ctx context.Context, runner CommandRunner, spec CommandSpec) ([]byte, error) {
	if runner != nil {
		return runner.CombinedOutput(ctx, spec)
	}
	cmd := exec.CommandContext(ctx, spec.Path, spec.Args...)
	cmd.Dir = spec.Dir
	if len(spec.Env) > 0 {
		cmd.Env = os.Environ()
		for key, value := range spec.Env {
			cmd.Env = upsertCommandEnv(cmd.Env, key, value)
		}
	}
	return cmd.CombinedOutput()
}

func upsertCommandEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}
