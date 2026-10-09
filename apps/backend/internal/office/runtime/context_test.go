package runtime

import "testing"

// TestRunContext_CanMutateTask_RefusesWildcardSentinelTarget covers a run
// whose own TaskID is the wildcard sentinel (a payload injecting
// task_id="*" stays task-bound per WithTaskScope's fail-closed narrowing,
// see context_builder.go#build). The sentinel must never satisfy the
// self-match check, since a caller can trivially reach that target by
// omitting an explicit task_id and letting it default to runCtx.TaskID.
func TestRunContext_CanMutateTask_RefusesWildcardSentinelTarget(t *testing.T) {
	ctx := RunContext{
		TaskID: WildcardTaskScope,
		Capabilities: Capabilities{
			CanUpdateTaskStatus: true,
		},
	}

	if ctx.CanMutateTask(WildcardTaskScope) {
		t.Fatal("a run whose own TaskID is the wildcard sentinel must not gain authority over a target literally named \"*\"")
	}
}
