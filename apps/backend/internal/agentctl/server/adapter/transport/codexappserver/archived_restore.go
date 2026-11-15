package codexappserver

import (
	"context"
	"errors"
	"strings"

	protocol "github.com/kandev/kandev/pkg/codexappserver"
)

// Resume the original provider identity before considering history fallback.
// Archive state is read from the provider; no rollout file is read or moved.
func (a *Adapter) resumeRetainedThread(ctx context.Context, client *protocol.Client, params protocol.ThreadResumeParams, response *protocol.ThreadStartResponse) error {
	err := client.Call(ctx, protocol.MethodThreadResume, params, response)
	if err == nil || a.cfg.Extra["allow_archived_thread_restore"] != "true" {
		return err
	}
	var stored protocol.ThreadReadResponse
	if readErr := client.Call(ctx, protocol.MethodThreadRead, protocol.ThreadReadParams{ThreadID: params.ThreadID}, &stored); readErr != nil ||
		stored.Thread.ID != params.ThreadID || !archivedNativeThreadPath(stored.Thread.Path) {
		return err
	}
	var restored protocol.ThreadReadResponse
	if restoreErr := client.Call(ctx, protocol.MethodThreadUnarchive, protocol.ThreadReadParams{ThreadID: params.ThreadID}, &restored); restoreErr != nil {
		return restoreErr
	}
	if restored.Thread.ID != params.ThreadID {
		return errors.New("unarchive Codex thread returned a different native identity")
	}
	return client.Call(ctx, protocol.MethodThreadResume, params, response)
}

func archivedNativeThreadPath(path *string) bool {
	if path == nil {
		return false
	}
	parts := strings.Split(strings.ReplaceAll(*path, "\\", "/"), "/")
	return len(parts) >= 2 && parts[len(parts)-2] == "archived_sessions"
}
