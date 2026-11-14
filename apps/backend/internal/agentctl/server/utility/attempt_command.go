package utility

import (
	"context"
	"os/exec"
)

type attemptContextKey struct{}
type attemptContext struct {
	owner *attemptOwner
	id    string
}

func withAttempt(ctx context.Context, owner *attemptOwner, id string) context.Context {
	return context.WithValue(ctx, attemptContextKey{}, attemptContext{owner: owner, id: id})
}

func changeAttempt(ctx context.Context, update func(*ownedAttempt)) {
	if a, ok := ctx.Value(attemptContextKey{}).(attemptContext); ok {
		a.owner.change(a.id, update)
	}
}

func startInferenceCommand(ctx context.Context, cmd *exec.Cmd) error {
	changeAttempt(ctx, func(a *ownedAttempt) { a.Status.State = AttemptStarting })
	if err := cmd.Start(); err != nil {
		changeAttempt(ctx, settleNotStarted)
		return err
	}
	changeAttempt(ctx, func(a *ownedAttempt) { a.PID = cmd.Process.Pid; a.Status.State = AttemptRunning })
	return nil
}

func settleNotStarted(a *ownedAttempt) {
	if a.PID != 0 {
		return
	}
	a.Status.Known, a.Status.Settled, a.Status.State = true, true, AttemptNotStarted
}

func markInferenceDispatch(ctx context.Context) {
	changeAttempt(ctx, func(a *ownedAttempt) { a.Status.PromptDispatch = PromptPossiblySent })
}

func finishInferenceCommand(ctx context.Context, terminated bool) {
	changeAttempt(ctx, func(a *ownedAttempt) {
		a.Status.Known, a.Status.Settled, a.Status.State = false, false, AttemptUnknown
		if terminated {
			a.Status.Known, a.Status.Settled, a.Status.State = true, true, AttemptTerminated
		}
	})
}
