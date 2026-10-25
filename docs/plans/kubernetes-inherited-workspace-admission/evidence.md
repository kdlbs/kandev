# Kubernetes inherited workspace investigation

Investigation date: 2026-10-09. Bug class: backend workspace/runtime admission,
with a browser-triggered workspace-only preparation path. No production code or
permanent tests changed. No native subagents or agent subprocesses were launched
for the disposable reproduction tasks.

## Executor verification

Profile `98ee6f2f-8f9e-4288-9edf-7140bc42f6b9` is "Dedicated Kubernetes agents",
owned by executor `a90681f8-2488-4b77-9dca-41828905f406`, type `k8s`.
Verified through `GET /api/v1/executors`, `GET /api/v1/executor-profiles`, then
`list_executor_profiles_kandev` with the actual executor UUID. The active MCP
catalog lacked executor listing/tool search; guessed built-in IDs returned no
profiles. No alternate executor was used.

Live backend: `v0.94.0-604-g059260b30`, commit
`059260b30fc68bbcbead629f7fa7e80f1ee0a5e8`.
Investigation checkout: `d7a44e96ede93d50c6277773f4174de716272f02`.
Logs establish live behavior; source tracing uses the newer checkout.

## Original incident

The caller-provided creation receipt records success at
`2026-10-09T10:54:23.982302315Z`, `start_agent=false`, omitted `workspace_mode`,
and inherited profile identity. Retained logs confirm task creation at
`10:54:24.010Z` and a description update at `10:56:16.762Z`.

At `10:57:12.787Z`, workspace preparation bound child
`fb5989a2-5508-4cbd-97f9-23f41821046f`, session
`6f340c72-ee7b-4a23-a666-f2457c1feb04`, to parent environment
`1490bd45-31d3-4773-a132-7cd85ff9b72a` (`source=parent_session`).
At `10:57:14.887Z` launch failed exactly:

```text
failed to create execution: workspace reuse is unsafe: task environment does not match the requested Kubernetes task
```

The stack reaches `PrepareTaskSession.func1` through `LaunchPreparedSession`.
The warning says "failed to launch workspace for prepared session"; the task
becomes FAILED. This was later workspace admission, not MCP creation failure
and not evidence of an agent subprocess starting.

The original child was deleted at `10:57:33.653Z`, before this investigation.
The parent's related-task list initially named replacement quick-terminal task
`88e356e3-ccfd-42e3-bb02-9d1a0ebbd87d`. Neither was mutated by this investigator.

## Paired disposable reproduction

All cases used workspace `9c1e60d1-2028-4810-aef4-ee3ad4a6b074`, repository
`bb5f06a0-ee0c-459d-a991-8cee3205254c`, base `main`, the verified executor/profile,
and agent profile `d2098913-e21a-4589-a1a7-ba4561b14a07`. Creation used
`parent_id="self"`, `start_agent=false`, and unique external IDs. The Kanban
depth resolver placed them under common parent
`455b8417-0979-4657-a7f0-800509852d12`, matching the original parent identity.

| Case | Task and session | Canonical environment | Result |
| --- | --- | --- | --- |
| Omitted mode, receipt resolves `inherit_parent` | Task `5d0e06c8-4270-4c5e-805a-4dd72095240c`; session `061e1080-2769-424a-8bd5-973a46967031` | Parent-owned `1490bd45-31d3-4773-a132-7cd85ff9b72a` | Creation succeeds at 11:04:32Z. Later browser preparation fails at 11:05:05.002Z with the exact original error. |
| Explicit `new_workspace`, first control | Task `62c58c54-ef5c-435a-9c7a-64321cdaa236`; session `c516d4f6-85ef-4877-a693-f1afbdcd8139` | Own `8b4dabe9-fba1-4239-9de1-4538643386b5` | Creation succeeds. Pod/bootstrap succeeds, but external archive during preparation prevents registration. Not a successful control. |
| Explicit `new_workspace`, uninterrupted control | Task `fede0d5e-c18a-4ba4-862b-90eab30e4fc8`; session `d8003513-4230-4958-9a02-257219441697` | Own `1fff2628-36b8-46d3-9c4c-421dfc57fd86` | Creation succeeds. Workspace preparation reaches `ready`; session remains CREATED. Archived by investigator after ready evidence at 11:08:18.835Z. |

For the uninterrupted control, the exact workspace-only request was:

```json
{
  "type": "request",
  "action": "session.launch",
  "payload": {
    "task_id": "fede0d5e-c18a-4ba4-862b-90eab30e4fc8",
    "intent": "prepare",
    "launch_workspace": true,
    "activation_source": "session_open"
  }
}
```

The response returned `success=true`, session `d8003513...`, state CREATED at
`11:06:53.814Z`. Later read-only task environment/session APIs confirmed a ready
`k8s` environment owned by this task, the same executor/profile, and CREATED
session state. Readiness was captured before archiving.

The first pair was opened and archived externally before the investigator's
prepare request. That request returned "task cleanup in progress" at 11:05:22Z.
This is a separate archive-related rejection; it does not replace the inherited
failure already captured at 11:05:05Z. The interrupted control also logged a
cleanup retry with a missing secret while bootstrap finished. It is excluded
from the proposed repair and needs separate evidence if pursued.

## Diagnosis and repair boundary

The parent API confirms environment `1490bd45...` is ready, type `k8s`, owned
by the parent task. Both child and parent select the same executor/profile.
The failing predicate is therefore differing task ownership, rather than an
executor-type or profile mismatch.

`internal/orchestrator/handoff_inheritance.go` intentionally propagates the
parent environment into the child session. Orchestrator executor admission
allows matching executor types. Kubernetes lifecycle's
`claimTaskKubernetesEnvironment` rejects `env.TaskID != original.TaskID` before
claiming runtime inventory. This matches the accepted task-owned compute and
credential trust boundaries. The defect is missing early unsupported-reuse
admission and recovery guidance at the task launch seam.

Reuse existing `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.4` and
`AC-EXECUTORS-KUBERNETES-TASK-POD-001.6`; no new product requirement is needed.
The system-design refinement and pending work order preserve defaulting,
deferred creation, task-owned pods, and explicit new-workspace selection.

## Evidence and limitations

Baseline backend bundle:
`.kandev/diagnostics/2a5b301dfa3274a62c97eec243c914d6.zip`, captured 11:02:52Z.
Comparison backend/frontend bundle:
`.kandev/diagnostics/a66647d6170ee93b9a1cb4dcc7b63592.zip`, captured 11:07:08Z.
Both manifests report ready and no warnings. Log timestamps are `+0100`;
this report normalizes them to UTC. Search exact task/session IDs within
`backend/backend-logs.log`, not stored agent conversations.

The repeated backend-only bundle request reused the earlier bundle. It cannot
prove later outcomes. The comparison bundle was collected with frontend evidence
to correlate the externally triggered opening/error toast. Final ready and
archive facts come from later bounded API snapshots, not that earlier ZIP.

No live database file is mounted or exported in this execution workspace;
historical facts come from retained logs and current state from read-only APIs.
Go is unavailable (`go: not found`), so no focused Go test was executed. Live
reproduction supplies the diagnostic evidence; permanent regression tests are
defined in the pending work order.

## Cleanup and design validation

All three disposable tasks are archived. No parent task or environment was
archived, stopped, or edited; the parent remained IN_PROGRESS. Temporary ZIP
extraction is removed; the two bounded bundles remain available as evidence.
No temporary instance, instrumentation, or reproduction test was created.
Physical Pod/PVC deletion is asynchronous; archive success alone is not proof
that cluster cleanup has completed.

Specification catalog validation passed (368 decisions, 1493 specifications).
Specification lint and `git diff --check` passed. The repository's
`validateCoverage` delivery validator returned `covered` with no errors for the
pending work order and its planned production boundary; all three requirement
IDs resolve through their referenced designs. This was a local working-file
preflight, not a submitted PR or product test run.
