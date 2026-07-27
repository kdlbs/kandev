---
name: pr-fixup
description: Wait for CI and automated reviews on a PR, fix valid failures and comments in the primary conversation, verify, and push.
---

# PR Fixup

Use this workflow directly in the user-started primary conversation. Do not
launch a verifier, implementer, or other remediation subagent. A read-only
`pr-poller` is the sole exception: launch it only when the user explicitly asks
to wait for or monitor PR updates. For a cost-controlled workflow, the user may
switch the same conversation to the lower-cost implementation/test model before
starting CI remediation.

Use `gh` by default; when it is unavailable after access is approved, use an
available GitHub integration. `scripts/pr-state`, `scripts/pr-resolve`, and
`scripts/run-quiet` are at the worktree root.

## Pipeline

Create a visible checklist:

1. Gather PR state
2. Fix failing CI checks
3. Triage review comments
4. Address valid comments
5. Commit, rerun affected checks, and push
6. Re-check the new head
7. Report

## 1. Gather PR State

Before the first GitHub helper call, request any runtime network approval that
the environment requires. If access is denied, cancelled, or interrupted, stop
the workflow permanently; retry only transient fetch failures after access is
approved.

Run `scripts/pr-state --summary <PR>` once. Record `checks_head_sha`,
`checks_snapshot_complete`, `failed_checks`, `pending_checks`,
`unresolved_review_thread_count`, `hidden_unresolved_threads`, and
`actionable_issue_comment_count`. Inspect mergeability separately through
`references/merge-conflicts.md`; it is not a `pr-state --summary` field. If a
named reviewer is the semantic evidence source, use `--trusted-reviewer` only
when `review_evidence.trusted_producer` is `"true"`; never use that shortcut
for forks, security, or architecture.
Treat `trusted_producer=true` as qualifying provenance only for the dedicated OpenCode App, never merely because a reviewer name matches.

For pending CI, do not run a rapid parent polling loop. Wait at a reasonable
interval, then run the same summary again. Stop after about 20 minutes and
report the exact pending checks as "CI in progress."

When the user explicitly asks to wait or monitor, the main conversation may
delegate one platform `pr-poller`. Give it the PR number and any selected
reviewer, then use its report only as status evidence. The poller is read-only,
time-bounded, and must not edit, push, comment, resolve threads, inspect source,
or spawn children. The primary conversation owns all triage and remediation;
never launch a poller automatically or relaunch one after a denied approval.

Treat the state as clean only when the current head has no failed or pending
checks, no merge conflict, no actionable review thread or issue comment, and
qualifying exact-head semantic evidence where PR delivery requires it.

## 2. Fix CI Failures

Before changing code, confirm every reported failed check and its `run_id`. Use
`scripts/run-quiet gh-run -- gh run view <run-id> --log-failed` so large logs do
not flood the conversation. Reproduce the exact failed command where possible;
CI-specific Go lint often needs `golangci-lint run ./... --new-from-rev=<base>
--timeout=5m`.

Fix with `/tdd` or `/e2e` as applicable, run focused checks, and keep each
remediation scoped to the reported failure. Do not suppress a failure or mark a
check clean without fresh evidence.

## 3. Triage And Address Reviews

Use `scripts/pr-resolve list <PR>` to obtain unresolved threads. For each
comment, decide whether it is valid, already addressed, a preference, or wrong
for this codebase. Validate against the current head, the spec, and existing
architecture before editing or replying.

Make only valid changes. For an invalid comment, reply with concrete reasoning
only when the user asks to respond. Resolve a thread only when the change or
response genuinely addresses it.

## 4. Commit, Verify, Push

Commit through `/commit`, then rerun only the unit, integration, or E2E command
affected by the remediation. Push when that targeted check passes for the exact
current `HEAD`. Run broad `/verify` only if the user explicitly requests it or
the PR/CI finding requires it.

## 5. Re-check

After every push, run `scripts/pr-state --summary <PR>` again for the new head.
Treat prior review evidence as stale. Repeat this workflow only for a new CI
failure or actionable current-head review finding; otherwise report the PR as
ready or CI as still in progress.

## Guardrails

- Do not create Kandev subtasks unless the user explicitly asks for task
  tracking.
- Do not use native delegation or a full-history context fork to poll CI.
- Do not push, post comments, or resolve threads when the user asked for review
  only.
- Do not proceed with an unverified PR when mandatory verification is blocked.
