# Backend Reproduction

Use this for backend-logic bugs: validation, dedup, data shaping, error mapping, identifier assignment, workflow routing, API/service behavior.

## Rule

Do not launch the UI for backend-logic bugs. Reproduce with a focused Go test against the real service path.

## Pattern

The office/task service has ready-made test harnesses such as `setupOfficeTest`. Write a throwaway `*_test.go` next to the suspect package and drive the real method:

```go
func TestRepro_DuplicateTitleRejected(t *testing.T) {
    svc, repo := setupOfficeTest(t)
    ctx := context.Background()
    _ = repo

    task, err := svc.CreateTask(ctx, &CreateTaskRequest{
        WorkspaceID: "ws-1",
        Title:       "Office Task",
        ProjectID:   "proj-1",
    })
    if err != nil {
        t.Fatalf("CreateTask: %v", err)
    }
    if task.Identifier == "" {
        t.Fatalf("expected identifier to be assigned, got empty")
    }
}
```

Run only the repro:

```bash
cd apps/backend && go test -tags fts5 -run TestRepro ./internal/task/service/ -v
```

## After Reproduction

- If the throwaway test proves the bug, hand off to `/fix` or `/tdd`.
- Convert the repro into a permanent regression test as part of the fix.
- Remove throwaway tests before finishing if they are not the final regression test.

## Root Cause Discipline

Before fixing, state:
- actual cause, not symptom;
- exact condition that triggers it;
- smallest input or state needed to reproduce;
- code path from input to failure.

## Workflow Transition Triage

For a partial-success report where a workflow step mutates state but sends no
prompt or message, inspect the serialized step and distinguish payload fields
from trigger actions: `prompt` supplies content, while `auto_start_agent`
dispatches it. Trace session identity separately; fixed-profile switches may
retain legacy implicit-launch behavior, while in-place mutations preserve the
session and do not satisfy a `sessionSwitched` gate. Verify the distinction with
paired focused service tests—with and without the dispatch action—that assert
both the state mutation and the resulting message/prompt output.
