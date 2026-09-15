---
id: "01-prompt-policy"
title: "Enforce final Git prompt policy"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-GIT-SUBPROCESS-ADMISSION-002
acceptance_criteria:
  - AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.1
  - AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.2
system_design:
  - ../../specs/platform/system-design/git-subprocess-execution.md
---

# Task 01: Enforce final Git prompt policy

## Summary

All runner variants deny interactive authentication after caller overrides; valid selected credentials and input protocols still work.

## In scope

Apply shared final preparation to every runner variant after environment assembly. Move the existing SSH normalization into the shared package.
Preserve explicit credential scope, indexed configuration, stdin, and working-directory semantics.
Integrate the merged dependency recorded in plan.md before this work begins.
Do not deduplicate repeated Git helper/configuration keys at different indexes; normalize only repeated assignments to the same environment variable.
Add an ordered block case with empty helper resets, repeated user helpers, hooks/notes entries, and a marker-owned host helper.
Assert that preparation preserves the entire indexed block and selected credential-directory variables.
Keep the host bridge's shell function unchanged; it is not an SSH wrapper.

Write `TestManagedGitCredentialFillPTY` first in `git_noninteractive_unix_test.go`.
Start a test subprocess under a controlling PTY, then invoke the production runner with real Git and a failing helper.
On the current code, assert that the test detects an actual waiting prompt and fails under an outer timeout.
Do not mistake Git's fatal message quoting a username prompt for an interactive read.

In `git_noninteractive_test.go`, add `TestGitFinalEnvironment`, `TestGitCredentialHTTPSelectedScope`, and `TestGitSSHCommandPreservation`.
Cover all runner variants, nil and explicit empty environments, duplicate keys, inherited interactive settings, and replacement of `cmd.Env` after construction.
Use a local HTTP Basic-auth Git fixture with distinct synthetic host and instance credentials.
Prove valid helper success, selected-scope denial without fallback, stdin preservation, and no secrets in captured errors or logs.
Cover quoted/direct SSH paths, `GIT_SSH`, inherited BatchMode=no, unsupported wrappers, and repeated preparation.
Native Windows coverage must execute deny-only askpass, not merely inspect its environment value.

## Out of scope

Credential-authority changes, transport fallback, live-instance changes, and unrelated refactors.

## Acceptance

- All runner variants deny interactive authentication after caller overrides; valid selected credentials and input protocols still work.
- Run new behavioral tests before implementation and record the expected failure, then the passing result.
- Preserve the contracts and exclusions in the linked design.

## Verification

```bash
git merge-base --is-ancestor 9cc146ee21c296d553ae684531219a5c711a4213 HEAD
(cd apps/backend && go test ./internal/common/subproc ./internal/gitconfigenv ./internal/agentctl/server/process -count=1 -timeout=5m)
```

Run the same applicable package tests on native Windows. Skip only Unix PTY assertions; record platform evidence separately.

## Files likely touched

- `apps/backend/internal/common/subproc/git_command.go`
- `apps/backend/internal/common/subproc/shared.go`
- `apps/backend/internal/common/subproc/git_noninteractive*.go (new)`
- `apps/backend/internal/agentctl/server/process/workspace_git_cmd.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_cmd_test.go`
- `apps/backend/internal/gitconfigenv/environment.go`

## Dependencies

Merged PR #3635 must be in HEAD; see the prerequisite in plan.md.

## Risks

Credential helper and process-lifecycle compatibility require real subprocess evidence. Do not infer success from environment assertions alone.

## Parallelism

`sequential`

## Inputs

- [Execution design](../../specs/platform/system-design/git-subprocess-execution.md).
- [Plan evidence and caller inventory](plan.md).
- Scoped AGENTS.md and relevant fix, TDD, and E2E skills.

## Results

Implemented on the verified PR #3635 descendant `9cc146ee21c296d553ae684531219a5c711a4213`.

Results: shared final preparation now runs at every classified Git runner boundary, preserving explicit environments, indexed Git configuration, selected credential scope, and direct OpenSSH options. Added PTY, local HTTP Basic-auth, runner-variant, duplicate-environment, SSH-normalization, and native Windows compile coverage. The Unix lifecycle tests were added ahead of Task 02 and pass against the current implementation.

Checks passed:

- `git merge-base --is-ancestor 9cc146ee21c296d553ae684531219a5c711a4213 HEAD`
- `(cd apps/backend && go test ./internal/common/subproc ./internal/gitconfigenv ./internal/agentctl/server/process -count=1 -timeout=5m)`
- `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o /tmp/kandev-subproc-windows.test ./internal/common/subproc`
- `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o /tmp/kandev-process-windows.test ./internal/agentctl/server/process`
- `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o /tmp/kandev-winproc-windows.test ./internal/agentctl/server/winproc`
