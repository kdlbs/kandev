# Session dialog E2E consolidation

Scope: `session/new-session-dialog.spec.ts`. Three cases repeat the initial
real agent launch, wait for its response, and open the same task. Move their
assertions into `creates second session and new tab appears`, which already
uses the same profile, prompt, repository and workflow.

| Original case | Preserved proof in the keeper |
| --- | --- |
| `+ dropdown lists sessions with status and primary indicators` | Retain the real Kanban entry, visible card, click and task URL assertion. Before launching the second session, open the menu and assert its first session row, `#1` label and `Agents` heading. Close the menu before the next step. |
| `opens dialog from + menu and shows environment info` | Open the dialog through the real menu and assert the dialog, environment information, prompt and start button before submitting. |
| `second session reuses same task environment` | Capture the non-null environment before navigation; after launching the second session, retain the two-session poll, non-null environment, unchanged environment ID and both session references. |

Each moved scenario retains its original title as a named test step. Keep the
original keeper's two tabs, loaded chat and exact backend session count. The
completed-session entry point, staged attachment, saved-prompt selection,
profile recency and cancellation with an unrelated workflow remain independent.
No mobile, timeout, retry, helper or production behavior changes are proposed.

The owner is `components/task/new-session-dialog.tsx` and the session runtime
attach path. The durable workspace contract is
[`additional-session-workspace-reuse`](../specs/tasks/requirements/additional-session-workspace-reuse.md).
Unit coverage for the dialog does not replace the retained real second launch.

## Existing instability

The unchanged file passed all nine cases with tracing and retries disabled.
In the earlier full 249-case shard reproduction, the dropdown case failed
before reaching the menu: its task appeared in the sidebar but was absent
from the Kanban board. The same case recovered on a CI retry in run
38432683722. The cause is unproven. Preserve its Kanban assertions in the
keeper; this consolidation does not claim to fix or bypass that observation.

## Verification

Run the complete candidate with the same build, one worker, tracing and no
retries, then verify all nine original scenario names remain as cases or
executed named steps. Local file timings are separate from hosted acceptance
of the 50% E2E runtime target.

The complete baseline passed 9/9; the candidate passed 6/6, with all three
moved steps verified in the passing Playwright traces. Neither run retried or
skipped a case. The sum of reported, rounded case durations fell from 95.1s
to 69.6s (approximately 26.8%). This is a single local file comparison,
not a whole-suite speedup. Raw logs, traces, original source and the scenario
identity receipt remain in ignored `e2e-focus/dialog-*`.
