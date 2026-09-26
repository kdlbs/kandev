---
id: "01-normalize-omp-shell-output"
title: "Normalize OMP structured final shell output"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/acp-shell-command-output.md"
parallelism: sequential
---

# Task 01: Normalize OMP Structured Final Shell Output

## Intent

Ensure a structured final ACP result marks stdout as committed only when Kandev actually recognized and normalized a stdout field. This lets the existing final command-echo strip run for OMP's cumulative content without adding an OMP-specific production branch.

## Acceptance

- OMP's adjacent `$ <command>` and real-output content persists only the complete real output, including its first byte when no separator exists.
- A recognized final stdout value is still normalized exactly once; legitimate real output beginning with `$ <command>` is not stripped a second time.
- The complete ACP adapter package passes.

## Verification

Run the new regression before the production change and confirm it fails for the echoed-command mismatch. After the minimal fix, run the focused regression and the repository backend test target:

```bash
cd apps/backend && rtk go test ./internal/agentctl/server/adapter/transport/acp -run 'TestConvertToolCallResultUpdate_OMPStructuredFinalOutputStripsCommandContent|TestNormalizeShellToolUpdateCommitsEchoStripExactlyOnceForRawOutputOnlyResult' -count=1
rtk make -C apps/backend test
```

## Files Likely Touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/shell_output.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/tool_call_update_test.go`
- `docs/specs/ui/requirements/acp-shell-command-output.md`
- `docs/plans/acp-shell-command-echo/plan.md`
- `docs/plans/acp-shell-command-echo/task-01-normalize-omp-shell-output.md`

## Dependencies

None.

## Parallelism

`sequential`. The production change and regression test exercise the same adapter state transition.

## Inputs

- OMP's ACP event mapper emits `$ <command>` as presentation content on command updates and merges result text after it.
- `NormalizeShellToolUpdate` currently sets `finalStdoutCommitted` for any non-nil `rawOutput`, even when `applyFinalShellResult` recognizes no stdout.
- ADR-0036 keeps provider output normalization at the ACP adapter boundary.
- The OMP regression scenario in `docs/specs/ui/requirements/acp-shell-command-output.md`.

## Output Contract

Report the root-cause branch changed, files modified, the red/green regression result, the full ACP package result, remaining risks, and synchronized task/plan statuses in this conversation.

## Results

Implemented the provider-neutral normalization-state repair.

- Changed `applyFinalShellResult` to report whether it recognized stdout.
- Used that result for `finalStdoutCommitted`, so metadata-only structured final
  results still receive the final command-echo strip.
- Added an adapter-level OMP regression for adjacent `$ <command>` and real
  output content with a structured metadata-only `rawOutput`.
- Red: the new regression failed because the persisted output retained the
  command echo directly before `README.md`.
- Green: the focused regression and existing exactly-once regression passed.
- Full ACP adapter package: 764 tests passed.

Remaining risk is limited to provider output that is absent before it reaches
the adapter; this change does not synthesize missing output or separators.

PR fixup verification:

- Review remediation commit `59f10560c8563a3fd68a5885cb555ecddaa302fc` added
  exit-code assertions, clarified the recognized-stdout wording, and aligned
  the verification command with the repository backend test target.
- Focused regressions passed: 2 tests.
- `rtk make -C apps/backend test` passed.
- At the exact PR head, CI reported 36 passed, 0 failed, and only `E2E Shard
  2/14` and `E2E Shard 6/14` still running. The review thread was resolved and
  the PR was mergeable; GitHub reported `UNSTABLE` only because CI was still
  in progress.
