---
id: "01-stream-startup"
title: "Preserve successful sign-in stream startup"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-ANTIGRAVITY-ACP-010
  - REQ-AGENTS-ANTIGRAVITY-ACP-015
acceptance_criteria:
  - AC-AGENTS-ANTIGRAVITY-ACP-010.4
  - AC-AGENTS-ANTIGRAVITY-ACP-010.5
  - AC-AGENTS-ANTIGRAVITY-ACP-015.1
system_design:
  - ../../specs/agents/system-design/antigravity-acp-agent.md
---

# Task 01: Preserve successful sign-in stream startup

## Summary

Fix the Antigravity sign-in caller's interpretation of `Client.StreamUpdates`.
Prove a healthy connection reaches initialization and native authentication,
while genuine setup failures and cancellation retain their failure behavior.

## In scope

- Add `TestAntigravityLoginStreamStartup` and `TestAntigravityLoginStreamFailure`
  in a new `antigravity_login_stream_test.go` using the real client and caller.
- Observe correlated initialize and `oauth-personal` authenticate requests on
  a local WebSocket, with channel-controlled handshake/response ordering.
- Demonstrate RED with the existing caller before changing production code.
- Call StreamUpdates directly; remove the wait helper and polling goroutine.
- Verify dial rejection, disconnect while initializing, and cancellation during
  a pending handshake. Preserve cleanup and sanitized error propagation.
- Record exact check results here and synchronize the plan and companion records.

## Out of scope

CLI migration, discovery/auth-store changes, generic client refactors, UI changes,
real Google consent, and live-instance mutation.

## Acceptance

1. A healthy fresh WebSocket reaches ACP initialize and `oauth-personal`
   authenticate without the false closure error or premature teardown.
2. Actual connection failure, disconnect, and cancellation fail cleanly and
   release resources; the prior argv and attempt-state tests still pass.
3. The regression fails on the old production caller and passes with the fix;
   package race tests, focused client tests, lint, and document checks pass.

## Verification

Run from the worktree root. Bootstrap the fresh worktree before any pnpm or hook
command. If mise has not trusted the copied configuration, use the already
reviewed root configuration through a scoped trusted-config environment.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -trimpath ./internal/agent/hostutility -run 'TestAntigravityLoginStream' -count=1 -v)
(cd apps/backend && go test -trimpath -race ./internal/agent/hostutility -count=1)
(cd apps/backend && go test -trimpath ./internal/agent/runtime/agentctl -run 'Test(StreamUpdates|HasAgentStream)' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agent/hostutility/antigravity_login.go apps/backend/internal/agent/hostutility/antigravity_login_stream_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the first focused Go command once before the correction and retain the
expected assertion failure, then rerun it after the correction. Use the repo's
PR-documentation coverage preflight to validate work-order traceability before
publication. Do not add broad verification or browser tests to this caller-only
repair. Assess public documentation impact with `/docs-maintainer` before delivery.

## Files likely touched

- `apps/backend/internal/agent/hostutility/antigravity_login.go`
- `apps/backend/internal/agent/hostutility/antigravity_login_stream_test.go` (new)
- `docs/specs/agents/system-design/antigravity-acp-agent.md`
- `docs/plans/antigravity-login-stream/plan.md` and this work order
- `docs/plans/antigravity-local-subscription/plan.md`
- `docs/plans/antigravity-local-subscription/task-03-google-sign-in.md`

## Dependencies

None. The full-argv correction from #5052 is already on the base branch.

## Risks

- An assertion limited to StreamUpdates returning nil misses caller behavior.
- Fixture WebSocket handlers must terminate on client cleanup, cancellation, and
  early failures; register cleanup before any fatal assertion.
- Native authentication and catalog refresh remain separate qualification work.

## Parallelism

`sequential`

## Inputs

- [Issue #5193](https://github.com/kdlbs/kandev/issues/5193).
- [Existing requirements](../../specs/agents/requirements/antigravity-acp-agent.md),
  REQ-AGENTS-ANTIGRAVITY-ACP-010 and REQ-AGENTS-ANTIGRAVITY-ACP-015.
- [Explicit local sign-in design](../../specs/agents/system-design/antigravity-acp-agent.md#explicit-local-sign-in).
- `antigravity_login_command_test.go`, `antigravity_login_test.go`, and client
  WebSocket tests in `internal/agent/runtime/agentctl/agent_test.go`.
- `.agents/skills/tdd/references/backend-tests.md`.

## Results

Implemented and verified on November 8, 2026.

- `configureAntigravityLoginInstance` opens its stream synchronously, preserves
  setup errors, and initializes only after connection setup succeeds. Removed
  the extra goroutine, polling helper, and false stream-closure interpretation.
- `TestAntigravityLoginStreamStartup` uses the production configure handler,
  actual client, and a channel-gated local WebSocket. It observes initialize
  followed by `oauth-personal` authenticate and verifies the connection stays
  live until explicit cleanup. RED failed with the exact reported closure error;
  GREEN passes.
- `TestAntigravityLoginStreamFailure` covers rejected handshake, disconnect
  during initialize, and cancelled pending setup. Cancellation uses the actual
  attempt-owner classifier, since closing a pending dial can return a network
  error rather than `context.Canceled`. No vendor binary or credentials are used.

Commands and results (working directories as in Verification):

| Command | Result |
| --- | --- |
| `pnpm install --frozen-lockfile` | Passed; fresh worktree dependencies installed. |
| `go test -trimpath ./internal/agent/hostutility -run 'TestAntigravityLoginStream' -count=1 -v` | Expected RED on old caller, then passed with the fix. |
| `TMPDIR=/private/tmp go test -trimpath -race ./internal/agent/hostutility -count=1` | Passed. |
| `go test -trimpath ./internal/agent/runtime/agentctl -run 'Test(StreamUpdates\|HasAgentStream)' -count=1` | Passed. |
| `bash scripts/lint-go-changed apps/backend/internal/agent/hostutility/antigravity_login.go apps/backend/internal/agent/hostutility/antigravity_login_stream_test.go` | Passed; zero issues. |
| `python3 scripts/list-docs.py validate` | Passed. |
| `python3 scripts/lint-spec-files.py --all` | Passed. |
| Local `.github/scripts/pr-docs.cjs` `validateCoverage` with the changed work orders and their current plan/requirement/design contents | Passed; covered, no errors. |
| `git diff --check` | Passed. |

The sandbox required a writable task-owned `GOCACHE` and
`GOLANGCI_LINT_CACHE` under `/private/tmp` and a process-scoped
`MISE_TRUSTED_CONFIG_PATHS` for this worktree. The first package race run hit
the existing cache-repair positive control because its descriptor-based path
walk rejects the macOS `/var` symlink. Using canonical `TMPDIR=/private/tmp`
made the complete package pass without changing that unrelated test or code.

Internal docs updated: the sign-in design, this package, and the original
subscription plan/Task 03. Public docs were checked; this caller repair changes
no setup action, configuration, CLI command, or rendered copy. The separate
browser/native qualification gates remain open, and no live Kandev state was
mutated. Screenshots are not required for this backend-only change.
