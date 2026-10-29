package process

import (
	"github.com/kandev/kandev/internal/common/launchwrapper"
	"github.com/kandev/kandev/internal/common/sandbox"
	"go.uber.org/zap"
	"os/exec"
	"slices"
	"time"
)

func prepareInteractiveCommand(proc *interactiveProcess) (*exec.Cmd, error) {
	req := proc.startReq
	// Build command - use Background context so the process lives beyond the request
	// The process lifecycle is managed by Stop() and wait(), not by context cancellation
	env := launchwrapper.StripControlCredentials(mergeEnvWithStrip(proc.startEnv, req.StripEnv))
	args := proc.startCmd
	if !req.LaunchWrapper.IsZero() {
		var err error
		args, err = launchwrapper.Prepare(req.LaunchWrapper, args, launchwrapper.Context{WorkDir: proc.startDir, Env: env})
		if err != nil {
			return nil, err
		}
	}
	if req.Sandbox != nil {
		args = slices.Clone(args)
		admitted, err := exec.LookPath(args[0])
		if err != nil {
			return nil, sandbox.ErrInvalidPolicy
		}
		args[0] = admitted
		args, env, err = sandbox.PrepareLaunch(req.Sandbox, args, env)
		if err != nil {
			return nil, err
		}
	}
	cmd := exec.Command(args[0], args[1:]...)
	if proc.startDir != "" {
		cmd.Dir = proc.startDir
	}
	cmd.Env = env

	return cmd, nil
}

func (r *InteractiveRunner) writeInitialCommand(proc *interactiveProcess) {
	req := proc.startReq
	// Wait for the shell to print its prompt before writing the initial command.
	// readOutput closes firstOutputCh on the first PTY read; without that gate a
	// heavy zsh/bash startup races the write and zsh ends up rendering the
	// command twice — once from PTY echo, once when it repaints with the
	// pending input. We still cap the wait so silent shells don't hang.
	if req.InitialCommand != "" {
		go func() {
			select {
			case <-proc.firstOutputCh:
			case <-time.After(2 * time.Second):
				r.logger.Warn("initial command write timed out waiting for shell prompt",
					zap.String("process_id", proc.info.ID))
			}
			// Brief settle so the prompt is fully painted before we type into it.
			time.Sleep(50 * time.Millisecond)
			proc.mu.Lock()
			pty := proc.ptmx
			proc.mu.Unlock()
			if pty != nil {
				_, err := pty.Write([]byte(req.InitialCommand + "\n"))
				if err != nil {
					r.logger.Warn("failed to write initial command to PTY",
						zap.String("process_id", proc.info.ID),
						zap.Error(err))
				} else {
					r.logger.Debug("wrote initial command to PTY",
						zap.String("process_id", proc.info.ID),
						zap.Bool("has_initial_command", req.InitialCommand != ""))
				}
			}
		}()
	}

}
