---
id: "07-compatibility-and-docs"
title: "Compatibility evidence and documentation"
status: pending
wave: 7
depends_on:
  - "06-conversation-forks"
plan: "plan.md"
requirements:
  - REQ-AGENTS-CODEX-NATIVE-001
  - REQ-AGENTS-CODEX-NATIVE-002
  - REQ-AGENTS-CODEX-NATIVE-003
  - REQ-AGENTS-CODEX-NATIVE-004
  - REQ-AGENTS-CODEX-NATIVE-005
  - REQ-AGENTS-CODEX-NATIVE-006
  - REQ-COSTS-CONVERSATION-USAGE-001
  - REQ-COSTS-CONVERSATION-USAGE-002
  - REQ-COSTS-CONVERSATION-USAGE-003
  - REQ-COSTS-CONVERSATION-USAGE-004
acceptance_criteria:
  - AC-AGENTS-CODEX-NATIVE-001.2
  - AC-AGENTS-CODEX-NATIVE-001.3
  - AC-AGENTS-CODEX-NATIVE-001.4
  - AC-AGENTS-CODEX-NATIVE-002.1
  - AC-AGENTS-CODEX-NATIVE-002.2
  - AC-AGENTS-CODEX-NATIVE-002.3
  - AC-AGENTS-CODEX-NATIVE-002.4
  - AC-AGENTS-CODEX-NATIVE-003.2
  - AC-AGENTS-CODEX-NATIVE-003.3
  - AC-AGENTS-CODEX-NATIVE-003.4
  - AC-AGENTS-CODEX-NATIVE-004.1
  - AC-AGENTS-CODEX-NATIVE-004.2
  - AC-AGENTS-CODEX-NATIVE-004.3
  - AC-AGENTS-CODEX-NATIVE-004.4
  - AC-AGENTS-CODEX-NATIVE-005.1
  - AC-AGENTS-CODEX-NATIVE-005.2
  - AC-AGENTS-CODEX-NATIVE-005.3
  - AC-AGENTS-CODEX-NATIVE-005.4
  - AC-AGENTS-CODEX-NATIVE-005.5
  - AC-AGENTS-CODEX-NATIVE-006.3
  - AC-COSTS-CONVERSATION-USAGE-001.1
  - AC-COSTS-CONVERSATION-USAGE-001.2
  - AC-COSTS-CONVERSATION-USAGE-001.3
  - AC-COSTS-CONVERSATION-USAGE-001.4
  - AC-COSTS-CONVERSATION-USAGE-001.5
  - AC-COSTS-CONVERSATION-USAGE-002.1
  - AC-COSTS-CONVERSATION-USAGE-002.2
  - AC-COSTS-CONVERSATION-USAGE-002.3
  - AC-COSTS-CONVERSATION-USAGE-002.4
  - AC-COSTS-CONVERSATION-USAGE-003.1
  - AC-COSTS-CONVERSATION-USAGE-003.2
  - AC-COSTS-CONVERSATION-USAGE-004.1
  - AC-COSTS-CONVERSATION-USAGE-004.3
system_design:
  - ../../specs/agents/system-design/codex-app-server.md
  - ../../specs/costs/system-design/conversation-usage.md
---

# Task 07: Compatibility evidence and documentation

## Summary and scope

Establish real protocol compatibility and publish accurate installation, diagnostic, flag, and usage guidance.
This work owns native/executor compatibility fixtures and documentation, not a generic QA pass.
Reconcile actual optional-event availability with the capability declaration.

## Exclusions

No broad unrelated test audit, ACP replacement, flag promotion, or publication to an external service.

## Likely files and ownership

- New `internal/agentctl/server/adapter/e2e/native_codex_test.go` live scenarios.
- Sanitized replay fixtures and version/schema metadata under native client/adapter tests.
- Focused native container/remote fake-server smoke tests and their fixture wiring.
- `docs/public/agents-and-profiles.md`, `agent-communication.md`, and `add-agent-cli.md`.
- `cmd/codexdbg/README.md`, native debug skill examples, scoped `agentctl/AGENTS.md`.
- Root/scoped guidance only where ACP-only descriptions become inaccurate.
- This plan's results, work-order results, and linked specifications.

## Acceptance

1. Live disposable 0.154.0 runs record initialization, prompt/resume, approvals, child-after-parent activity, multi-response usage, and fork behavior.
2. Executor and database checks provide actual results, and missing optional native fields produce documented degraded behavior instead of false capability claims.
3. Documentation explains profile coexistence, restart-required disablement, debug commands, raw capture handling, and cost provenance without requiring a plugin.

## Verification

Add the named live test suite so it creates and cleans up its own workspace.
The suite must fail with an actionable preflight error when explicitly enabled without required authentication.
Never point it at the user's current native thread.
Run from the repository root:

```bash
(cd apps/backend && KANDEV_CODEX_APP_SERVER_E2E=1 go test ./internal/agentctl/server/adapter/e2e -run '^TestNativeCodexAppServer' -count=1 -v)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --project containers tests/docker/codex-app-server.spec.ts tests/ssh/codex-app-server.spec.ts tests/kubernetes/codex-app-server.spec.ts)
(test -n "${KANDEV_TEST_POSTGRES_DSN:-}" && cd apps/backend && go test ./internal/task/repository/sqlite -run 'TestPostgres.*(Usage|Native|Fork)' -count=1 -v)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
python3 scripts/lint-harness-files.test.py
python3 .github/scripts/lint-harness-files.py --all
git diff --check
```

Set `KANDEV_TEST_POSTGRES_DSN` before the database command; the final pass also covers Task 06 fork migrations.
If the database is unavailable or tests skip, record the blocker and leave database validation pending.
Container scenarios must cover native command launch and profile gating in Docker, SSH, and Kind through existing fixtures.
Capture stderr only explicitly; sanitize fixtures before checking them in.
Record actual response-event availability, amount-unit evidence, and provider-estimate availability separately.
A missing optional estimate is acceptable; inventing its units is not.

## Risks

Authentication and remote/container infrastructure can block live evidence.
Do not mark the package implemented when required checks remain unrun.
Public docs must describe verified behavior, not this package's intended behavior.

## Results

The native Codex E2E suite passed against Codex 0.154.0. It covered initialization, prompt, resume, fork, fork workspace, and background completion. PostgreSQL usage and migration tests passed on PostgreSQL 16.

Docker, SSH, and Kind executor tests passed with a fake app-server. They check native command launch and profile gating through each executor. They do not run Codex inside those executors.

Direct app-server `item/tool/requestUserInput` requests now route through Kandev clarification controls. Fake-server adapter tests and desktop/phone E2E cover the path, including choice-only questions. The authenticated live run did not produce a question or approval request from a disposable write. Usage fell back to `turn_fallback`; exact response usage was not observed. A second provider thread appeared, but the run did not establish child-to-collaboration-call correlation. Live tests therefore do not establish direct-question or approval compatibility, exact response usage, child correlation, or all fork and background behavior.

The implementation follow-up and PR delivery are complete at head `4795ed8244184e421d3a7d2a6b8379e1f15628f9`. Exact-head CI passed all 60 checks, with no pending checks or unresolved review threads; GitHub reports the PR mergeable/clean. This does not close this work order: no live runtime check established provider-estimate availability for the tested thread, direct-question or approval behavior, exact response usage, child-to-collaboration-call correlation, or Codex compatibility inside Docker, SSH, and Kind. The executor matrix used a fake app-server. Keep the work order pending until the remaining acceptance evidence is recorded.
