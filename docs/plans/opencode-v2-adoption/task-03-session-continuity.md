---
id: "03-session-continuity"
title: "Preserve sessions and document compatibility"
status: completed
wave: 3
depends_on:
  - "02-migration-dialog"
plan: "plan.md"
requirements:
  - REQ-AGENTS-OPENCODE-V2-001
  - REQ-AGENTS-OPENCODE-V2-002
acceptance_criteria:
  - AC-AGENTS-OPENCODE-V2-001.1
  - AC-AGENTS-OPENCODE-V2-001.4
  - AC-AGENTS-OPENCODE-V2-001.9
  - AC-AGENTS-OPENCODE-V2-001.11
  - AC-AGENTS-OPENCODE-V2-002.1
  - AC-AGENTS-OPENCODE-V2-002.2
  - AC-AGENTS-OPENCODE-V2-002.3
  - AC-AGENTS-OPENCODE-V2-002.4
system_design:
  - ../../specs/agents/system-design/opencode-v2-adoption.md
---

# Task 03: Preserve sessions and document compatibility

## Summary

Preserve saved OpenCode conversations across adoption and external native upgrades.
Add permanent application and real ACP evidence, and publish the supported migration boundaries with the feature.

## In scope

- Preserve native ID, working directory, executor home, and Kandev identity when restoring through v2.
- Prevent silent new-session fallback after an OpenCode restore error.
- Verify/adjust v2 capability normalization, MCP injection, model selection, permissions, and process cleanup while retaining v1 behavior.
- Deterministic lifecycle failure coverage and isolated real-binary v1-to-v2 conversation tests.
- Scoped public documentation and agentctl guidance updates reflecting delivered behavior.
- Reconcile package results, new specification statuses where appropriate, and actual compatibility evidence.

## Out of scope

- HTTP provider rewrite, generic adapter redesign, plugin conversion, data rollback, and unrelated provider changes.

## Acceptance

1. An existing conversation continues with its prior native ID and history after migration; failure retains its Kandev identity and never synthesizes another conversation.
2. Real v1/v2 runs validate continuation plus the v2 capabilities Kandev uses, with isolated files and no orphan processes; remote support claims match actual evidence.
3. Public guidance explains fresh defaults, shared DB choice, unchanged standalone CLI, external-process boundary, supported session continuity, and post-activation failure limits.

## Verification

The work order adds the named test files and a disposable real-binary fixture.
The fixture acquires exact v1 1.18.32 and v2 2.0.18 through npm into test-owned storage with isolated HOME/XDG/workspace.
It must fail with a clear prerequisite error if an explicitly requested real run cannot start; a skipped test is not passing evidence.
Use a test-controlled provider endpoint where supported; otherwise document a configured test account without logging credentials.
Do not depend on an unversioned free-model availability promise.

```bash
(cd apps/backend && go test ./internal/agent/runtime/lifecycle ./internal/agent/agents ./internal/agentctl/server/adapter/...)
(cd apps/backend && go test -tags=e2e -run '^TestOpenCodeACP_(V1ToV2Resume|V2Compatibility)$' -count=1 -v -timeout 10m ./internal/agentctl/server/adapter/e2e)
(cd apps/backend && go test ./internal/agent/managedruntime -run '^TestOpenCodeSelectionPersistsAcrossStoreReopen$' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

The real suite must preserve a v1 turn, close its process, load its ID in v2, then ask a history-dependent question.
Include native 1.18.5 as an additional fixture when validating the previously observed user-installed path.
Verify local and HTTP MCP, permission responses, model selection, and process cleanup using deterministic fixtures where possible.
Task 01 already owns executor command contracts. Run available SSH/container integration fixtures if those paths change;
record platform gaps explicitly and do not claim real remote validation from host tests.
Rerun a prior task's targeted checks only when this work changes its covered behavior.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/session.go`; new `opencode_migration_test.go`.
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go` and related tests if normalization needs changes.
- `apps/backend/internal/agentctl/server/adapter/e2e/opencode_acp_test.go`; new `opencode_v2_migration_test.go` and isolated fixture helpers.
- Existing OpenCode MCP configuration strategy and its tests, only if real compatibility evidence requires changes.
- `apps/backend/internal/agent/managedruntime/opencode_selection_persistence_test.go` verifies the activated choice survives a database reopen.
- `docs/public/agents-and-profiles.md` (how-to migration and runtime reference); inspect `docs/public/windows-support.md`, `README.md`, and `docs/screenshots.md` for affected claims.
- `apps/backend/internal/agentctl/AGENTS.md` and `apps/backend/internal/agent/agents/ACP_BRIDGE_VERSIONS.md` when their documented contracts change.
- This package, linked requirements/designs, and amendment references.

## Dependencies

Tasks 01 and 02: resolved command source and real migration activation boundary.

## Risks

Upstream plugin behavior or data migration can fail after activation. Report that phase accurately and preserve session identity.
A basic prompt cannot prove history continuity; a mock continuation cannot prove upstream storage compatibility.
Provider or platform limitations may require recorded release blockers rather than fabricated passing evidence.

## Parallelism

`sequential`

## Inputs

- [Design](../../specs/agents/system-design/opencode-v2-adoption.md): Session continuity, Failure boundaries, Verification.
- Existing ACP adapter e2e harness, lifecycle restoration tests, and session browser fixtures.
- Earlier experiment evidence in the requirements; it supplements but does not replace these checks.
- `/docs-maintainer` guidance for public documentation updates.

## Results

Preserved OpenCode saved-session identity and made OpenCode restore failures return to Kandev without creating a replacement session. Updated compatibility guidance and added isolated upstream protocol evidence.

- The complete lifecycle package passed with a task-owned short `TMPDIR`; this avoids the Unix socket path limit encountered with the longer default test directory.
- `TestOpenCodeACP_V1ToV2Resume` and `TestOpenCodeACP_V2Compatibility` passed with exact managed versions 1.18.32 and 2.0.18, an isolated home/workspace/database, a local test provider, and confirmed process shutdown.
- The SQLite selection-reopen test passed; OpenCode restore-failure tests confirm that `agent.session.new` is not sent after load failure.
- Public documentation, coverage mapping, agentctl guidance, and locale validators passed.
- The real protocol fixture validates managed v1-to-v2 continuity. Native 1.18.5 command selection is covered by deterministic fixtures; no real native installation or remote executor was claimed.
