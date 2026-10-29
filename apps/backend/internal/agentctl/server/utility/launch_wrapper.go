package utility

import (
	"errors"
	"fmt"
	"os/exec"
	"slices"

	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/common/launchwrapper"
	"github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/pkg/agent"
)

func prepareACPLaunch(cfg *InferenceConfigDTO, admitted string, args, env []string) ([]string, error) {
	if !cfg.LaunchWrapper.IsZero() && (len(cfg.CommandPrefix) != 0 || (cfg.Protocol != "" && cfg.Protocol != agent.ProtocolACP)) {
		return nil, launchwrapper.ErrInvalid
	}
	inner := append(slices.Clone(args), cfg.CLIFlags...)
	inner[0] = admitted
	if !cfg.LaunchWrapper.IsZero() {
		return launchwrapper.Prepare(cfg.LaunchWrapper, inner, launchwrapper.Context{WorkDir: cfg.WorkDir, Env: env})
	}
	if err := managedruntime.PrepareNPMProjectPrefix(inner); err != nil {
		return nil, err
	}
	if len(cfg.CommandPrefix) == 0 {
		return inner, nil
	}
	prefix := slices.Clone(cfg.CommandPrefix)
	prefix[0] = resolveACPCommandPrefix(prefix[0])
	if prefix[0] == "" {
		return nil, errors.New("profile command prefix is unsupported")
	}
	return append(prefix, inner...), nil
}

func utilityStartError(cfg *InferenceConfigDTO, err error) string {
	if !cfg.LaunchWrapper.IsZero() || cfg.Sandbox != nil {
		return "wrapped utility process failed to start"
	}
	return fmt.Sprintf("start: %v", err)
}

// prepareACPProcess applies the sandbox after the inner profile command is admitted.
func prepareACPProcess(cfg *InferenceConfigDTO, admitted string, args, env []string) ([]string, []string, error) {
	argv, err := prepareACPLaunch(cfg, admitted, args, env)
	if err != nil {
		return nil, nil, err
	}
	if cfg.Sandbox != nil {
		if cfg.SandboxStarted == nil || cfg.SandboxStarting == nil || cfg.SandboxStartFailed == nil {
			return nil, nil, sandbox.ErrInvalidPolicy
		}
		argv[0], err = exec.LookPath(argv[0])
		if err != nil {
			return nil, nil, sandbox.ErrInvalidPolicy
		}
	}
	return sandbox.PrepareLaunch(cfg.Sandbox, argv, env)
}
