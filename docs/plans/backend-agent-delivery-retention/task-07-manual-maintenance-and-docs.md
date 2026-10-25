---
id: "07-manual-maintenance-and-docs"
title: "Integrate offline maintenance and document compatibility"
status: complete
wave: 7
depends_on:
  - 06-worker-and-operations
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DELIVERY-RETENTION-005
  - REQ-PLATFORM-DELIVERY-RETENTION-006
acceptance_criteria:
  - AC-PLATFORM-DELIVERY-RETENTION-005.4
  - AC-PLATFORM-DELIVERY-RETENTION-006.3
  - AC-PLATFORM-DELIVERY-RETENTION-006.4
  - AC-PLATFORM-DELIVERY-RETENTION-006.5
system_design:
  - ../../specs/platform/system-design/backend-agent-delivery-retention.md
---

# Task 07: Integrate offline maintenance and document compatibility

## Summary

Expose safe durable-delivery categories through the existing SQLite maintenance command and explain actual retention, disk and rollback behavior. Keep rendered UI unchanged.

## In scope

- Extend Analyze/Report and Run/ExecutionResult for bounded delivery categories using the same eligibility predicates and post-lock revalidation.
- Preserve read-only/no-migration dry-run, exclusive ownership, verified pre-execute backup and optional staged compaction. Report partial bounded execution accurately.
- Test legacy schema dry-run, current schema dry-run, repeated execute, protected partial terminals, cancellation/partial completion and compaction failure after successful deletes.
- Update public operations/configuration/CLI/session guidance and backend scoped AGENTS metric/persistence ownership with implemented facts only.
- Document startup defaults, expiry, automatic cleanup, protected uncertainty, explicit deletion and physical reclaim. Name production logs and dev-only expvar; the offline CLI remains SQLite-only.
- Document release N reader/writer expansion and N+1 pruning activation, tested N rollback, SQLite version-change VACUUM INTO snapshots (only two retained; preserve an external verified copy), and mandatory verified PostgreSQL pg_dump before N+1 upgrade with tested restore. Never claim a pre-retention binary is safe after pruning.
- Explain admitted SQL-work versus native SQLite lock-wait/Stop latency; do not promise 250 ms wall-clock completion during contention. Keep stable startup configuration unchanged by the scheduling implementation.
- Check root README and screenshots; no new UI/control, localization or screenshot replacement is expected.

## Out of scope

Implementation was authorized by the user on 2026-10-09.
Agentctl journal policy, canonical-history expiry, new UI and feature flags are excluded.
This order does not authorize commit, push, merge or delegation.

## Acceptance

- Read-only analysis changes no schema/data; execute requires ownership and verified backup, then shares live cleanup predicates and budgets.
- Logical removal, reusable pages, compaction, WAL/backups and PostgreSQL operator vacuum behavior have distinct reporting and public wording.
- Docs name the tested release N rollback artifact before N+1 activation. Missing real N compatibility evidence blocks release; existing desktop/phone recovery remains the user entry point.

## TDD evidence

Start each changed-logic scenario with a failing test against the pre-change behavior. Record Red, Green and refactor results.
Use existing backend test helpers, fake clocks and deterministic barriers. Do not use sleeps to manufacture race order.
Required new test groups: `TestDeliveryRetentionMaintenance`.
Names are planned deliverables, not existing passing tests. Before using a -run selector, list tests and confirm every required group exists.
The real repository and retained journal must prove state transitions where named; fakes do not prove database crash semantics.
Read `.agents/skills/tdd/SKILL.md` and its `references/backend-tests.md` at implementation time.

## Verification

Run from repository root. Commands are implementation-time checks, not run during planning.

```bash
(cd apps/backend && go test -trimpath -race ./internal/maintenance -count=1)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node docs/plans/backend-agent-delivery-retention/validate-package.cjs
git diff --check
```

Offline CLI is SQLite-only by existing contract. PostgreSQL automatic retention is tested in Tasks 03-06 and 08; do not claim the CLI can compact PostgreSQL.
For every environment-gated test, record executed and skipped subtests. A skip cannot complete the corresponding acceptance.
The package helper checks document links/IDs; it does not prove runtime behavior.

## Files likely touched

- `apps/backend/internal/maintenance/report.go`
- `apps/backend/internal/maintenance/run.go`
- `apps/backend/internal/maintenance/execute.go`
- `apps/backend/internal/maintenance/maintenance.go`
- `apps/backend/internal/maintenance/run_test.go`
- `docs/public/operations.md`
- `docs/public/configuration.md`
- `docs/public/cli.md`
- `docs/public/sessions-and-review.md`
- `apps/backend/AGENTS.md`

Add focused `_test.go` files beside changed logic; split oversized tests rather than extending files past lint limits.
Paths marked new are intended deliverables. Other directory references identify the existing integration boundary.

## Dependencies

Complete `06-worker-and-operations` first. Execute sequentially in the primary session.

## Inputs

- [Requirements](../../specs/platform/requirements/backend-agent-delivery-retention.md).
- [Design](../../specs/platform/system-design/backend-agent-delivery-retention.md); sections matching this scope.
- [Plan](plan.md), including baseline, storage thresholds and engine gates.
- Scoped `apps/backend/AGENTS.md`, existing delivery tests and nearby maintenance patterns.

## Risks

Limited candidate samples must not masquerade as exact database-wide totals. Existing CLI categories use one transaction; delivery batches require explicit partial-success reporting.

## Parallelism

`sequential`. No delegation is authorized.

## Results

Completed the existing bounded offline maintenance integration and six retention scenarios. Analysis stays read-only on current and legacy schemas. Execution verifies a backup before schema initialization, shares live retention predicates, preserves partial terminal evidence, and reports bounded progress across repeat invocations and compaction failure.

The resumed full maintenance race run exposed an undefined test variable and an incorrect compacted-row assertion. The latter now asserts the committed retirement checkpoint and absent expired event after compaction failure. No maintenance behavior change was needed. `go test -trimpath -race ./internal/maintenance -count=1` passed (16.002 seconds).

Updated operations, configuration, CLI and session guidance, plus backend AGENTS ownership. Root README and screenshot catalog have no changed surface or outdated retention claim, so neither needs a screenshot replacement. Public docs describe bounded samples and partial delivery execution separately from the legacy category transaction. Release N remains conditional on Task 08 binary evidence, never an unverified rollback claim.

`node --test scripts/validate-public-docs.test.mjs`, `node scripts/validate-public-docs.mjs` (47 pages), catalog validation, full spec lint, package structure/coverage (22/22 ACs, 65 links), and `git diff --check` passed. The resumed status correction changed the design from invalid `active` to `current`. Logs are retained under `/tmp/kandev-retention-resume-20261009/`.
