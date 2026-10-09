---
id: "05-parity-and-docs"
title: "Prove parity and publish the contract"
status: done
wave: 5
depends_on:
  - "04-retire-journal"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-002
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-005
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-006
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-007
acceptance_criteria:
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.1
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.4
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.7
  - AC-PLUGINS-PROMPT-HISTORY-HOST-005.1
  - AC-PLUGINS-PROMPT-HISTORY-HOST-005.2
  - AC-PLUGINS-PROMPT-HISTORY-HOST-005.3
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.3
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.4
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.5
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.6
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.7
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.8
  - AC-PLUGINS-PROMPT-HISTORY-HOST-007.4
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.9
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.10
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.11
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.12
system_design:
  - "../../specs/plugins/system-design/conversation-source-reconciliation.md"
---

# Task 05: Prove parity and publish the contract

## Summary

Complete the replacement scenario matrix on desktop and mobile, and synchronize supported API and upgrade documentation.
Record actual check results and promote the draft design only after all work orders pass.

## In scope

- Own final fixture scenarios for live ID updates, loaded-range deletion, reconnect, restart, malformed batches, access loss, and suspended tabs.
- Assert zero history rereads for normal streaming and irrelevant changes after hydration, on desktop and mobile.
- Prove missing revision coverage triggers repair even when a later event and revision check arrive.
- Document no content hashes, transient complete change batches, coverage-only messages, and recovery-only broader reads.
- Preserve alias, favorite, native navigation, and existing core panel tests.
- Update PLUGIN-API.md Host-only wire sections and public authoring/upgrade documentation at implementation cutover.
- Update scoped engineering guidance and all companion plan references/results that describe the retired transport.

## Out of scope

- External plugin creation/publication, built-in panel removal, saved-layout migration, and generic full-repository QA.

## Acceptance

- Desktop and mobile fixtures prove incremental updates without broad rereads, plus discrepancy recovery, through the unchanged public SDK.
- All work-order checks have recorded results. No stale replay or fixed-snapshot promise remains in the active reference.
- Documentation describes original-record reads, recovery semantics, logical cleanup, physical compaction, and backup-based downgrade.

## Verification

Run from the repository root. Obtain behavioral RED evidence before production edits.

```bash
(cd apps/web && pnpm e2e:run --project chromium tests/plugins/conversation-recovery.spec.ts tests/plugins/prompt-history-plugin.spec.ts tests/task/prompt-history-panel.spec.ts tests/session/session-stream-overload-isolation.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/plugins/mobile-prompt-history-plugin.spec.ts tests/session/mobile-session-stream-overload-isolation.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/plugin-sdk test && pnpm --filter @kandev/plugin-sdk typecheck)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Before the first pnpm command in a fresh worktree, run `(cd apps && pnpm install --frozen-lockfile)`.
Managed E2E rebuilds the application and fixture. Run desktop and mobile sequentially.

## Files likely touched

- `apps/web/e2e/tests/plugins/conversation-recovery.spec.ts`
- `apps/web/e2e/tests/plugins/prompt-history-plugin.spec.ts`
- `apps/web/e2e/tests/plugins/mobile-prompt-history-plugin.spec.ts`
- `apps/web/e2e/fixtures/plugins/prompt-history-plugin/bundle.js`
- `apps/web/e2e/helpers/causal-waits.ts`
- `apps/backend/internal/office/testharness/routes.go`
- `docs/plans/plugins/PLUGIN-API.md`
- `docs/public/plugins-authoring.md`
- `docs/public/plugins-manifest.md`
- `docs/public/operations.md`
- `apps/backend/AGENTS.md`
- `apps/web/AGENTS.md`
- `docs/specs/plugins/system-design/conversation-source-reconciliation.md`
- `docs/plans/conversation-storage-replacement/`

## Dependencies

04-retire-journal.

## Risks

- A browser clock cannot simulate server expiry or restart. Use isolated harness controls and causal completion.
- An old frontend must not silently accept unsupported v1 ACK semantics.
- Do not replace historical validation counts with claims that the new implementation passed them.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/prompt-history-extraction-host.md) and the frontmatter criteria.
- [System design](../../specs/plugins/system-design/conversation-source-reconciliation.md).
- [ADR](../../decisions/2026-09-16-conversation-source-reconciliation.md).
- Existing conversation handler, journal, Host facade, and recovery tests provide the regression patterns.

## Results

- Updated the active Host API, public plugin authoring/manifest/operations references, source reconciliation design, ADR, and companion plans for the current-state v2 contract.
- Replaced the obsolete expiry mutation E2E seam with a source-change drop and recovery assertion.
- Desktop managed E2E passed 6/6 for core/plugin recovery, plugin parity, core panel behavior, and stream isolation.
- Mobile managed E2E passed 2/2 for prompt-history touch navigation and stream isolation.
- Frontend unit tests passed 64/64; TypeScript, ESLint, plugin SDK tests/typecheck, i18n checks, public-doc validation, specification validation, SQL guard, and `git diff --check` passed.
- The follow-up completed the dependent PostgreSQL, Office, task-service, and final recovery gates. The final guarded browser matrix passed desktop 5/5 and mobile 5/5, with 41 focused frontend tests and gateway race coverage passing on revision `213492517315abb38697b765e91e6fe0ea5388c7`. The historical 6/6 desktop and 2/2 mobile counts above remain unchanged.


## Large legacy SQLite verification

The requested large-database verification passed on a 2.95 GB synthetic legacy
database. Backup, cleanup, task-repository initialization, and HTTP readiness
were measured separately. Both 60-second streaming runs completed without
write or readiness/probe failures. Active SQLite triggers, production workers,
and retired host files were audited. Manual compaction and subsequent readiness
passed; no automatic startup VACUUM was added.

See [the verification report](verification/large-sqlite-upgrade.md) for workload,
commands, measurements, evidence, and limits. The public operations guide now
includes explicit post-upgrade compaction. PostgreSQL and other existing plan
gates remain open. All changes remain uncommitted.

## Remaining-gate handoff

The follow-up package records the final PostgreSQL, backend, and recovery
evidence. Existing results remain historical.
