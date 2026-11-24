---
id: "04-user-flows-and-docs"
title: "Prove user flows and document behavior"
status: done
wave: 4
depends_on:
  - "03-npm-and-executor-proof"
plan: "plan.md"
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-007
acceptance_criteria:
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.2
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.3
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.4
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.5
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.11
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.12
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.13
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.14
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.15
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.16
system_design:
  - ../../specs/agents/system-design/managed-npm-repair-reuse.md
---

# Task 04: Prove user flows and document behavior

## Summary

Prove the first-recovery and later-first-attempt experience on desktop and phone.
Publish the supported behavior and synchronize the implementation records.

## In scope

- Existing session and settings flows with real isolated backend/agentctl lifetimes.
- Same native conversation, preserved transcript, one prompt delivery, and matching discovery results.
- First-recovery progress, later ordinary resume, restart, failures, and old-peer behavior.
- Public agent runtime guidance and accurate capability/storage limits.
- Work-order results, specification lifecycle, targeted lint, and documentation coverage.

## Out of scope

- New rendered UI, copy, controls, screenshots, new settings, and broad QA or verification passes.

## Acceptance

1. Desktop and phone scenarios show recovery once, then compatible cold resume
   on the first attempt. They preserve conversation/transcript, usable input,
   and one follow-up prompt after a real backend/agentctl restart.
2. Discovery/profile scenarios reuse compatible files while preserving saved
   and draft values. Failure and cancellation retain bounded attempts and existing accessible controls.
3. Public guidance describes reusable repairs and their execution-host lifetime.
   Exact checks and remaining platform evidence appear in the work-order results.

## Presentation and mobile

No rendered UI change is planned. Reuse the current task transcript and profile
composition described by the plan. No ASCII preview is required.
Assert existing phone containment and recovery controls when the failure path runs.
Update the design, preview, and locale scope before any unexpected UI change.

## Verification

Run sequentially from the repository root. Use the managed runner and one shard.
For a fresh worktree, first run `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/session/managed-npm-cache-recovery.spec.ts tests/settings/managed-npm-cache-recovery.spec.ts --grep 'reusable verified cache repair')
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/session/mobile-managed-npm-cache-recovery.spec.ts tests/settings/mobile-managed-npm-cache-recovery.spec.ts --grep 'reusable verified cache repair')
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/session/managed-runtime-startup-retry.spec.ts tests/settings/profile-capability-discovery.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run `bash scripts/lint-go-changed` with the exact changed Go paths from Tasks 01-03.
Run this documentation-coverage preflight for the full package.
The projected production path also exercises the gate during design-only validation.

```bash
node - <<'JS'
const fs = require('node:fs');
const path = require('node:path');
const cp = require('node:child_process');
const {validateCoverage} = require('./.github/scripts/pr-docs.cjs');
const dir = 'docs/plans/managed-npm-repair-reuse';
const packagePaths = fs.readdirSync(dir).filter(n => n.endsWith('.md')).map(n => path.posix.join(dir, n));
const contracts = [
  'docs/specs/agents/requirements/managed-npm-repair-reuse.md',
  'docs/specs/agents/requirements/managed-npm-runtime-recovery.md',
  'docs/specs/agents/requirements/managed-npm-cache-recovery.md',
  'docs/specs/agents/system-design/managed-npm-repair-reuse.md',
  'docs/specs/agents/system-design/managed-npm-runtime-recovery.md',
  'docs/specs/agents/system-design/managed-npm-cache-recovery.md',
];
const tracked = cp.execFileSync('git', ['diff', '--name-only'], {encoding: 'utf8'}).trim().split('\n').filter(Boolean);
const added = cp.execFileSync('git', ['ls-files', '--others', '--exclude-standard'], {encoding: 'utf8'}).trim().split('\n').filter(Boolean);
const docs = [...new Set([...packagePaths, ...contracts, ...tracked, ...added])].filter(p => p.endsWith('.md'));
const fileContents = Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')]));
const changedFiles = [...new Set([...packagePaths, ...contracts, ...tracked, ...added]), 'apps/backend/internal/agent/managedruntime/prepared_runtime.go'].map(filename => ({filename, status: added.includes(filename) ? 'added' : 'modified'}));
const result = validateCoverage({changedFiles, fileContents});
console.log(JSON.stringify({ok: result.ok, status: result.status, workOrders: result.workOrders, errors: result.errors}, null, 2));
if (!result.ok) process.exitCode = 1;
JS
```

Record the lint invocation and all verification results before completion.

## Files likely touched

- `apps/web/e2e/helpers/managed-npm-cache-recovery-scenarios.ts` and existing settings scenario helpers.
- Existing session/settings cache-recovery desktop and mobile spec files.
- `docs/public/agents-and-profiles.md`; task guidance only where its runtime wording changes.
- `docs/specs/agents/requirements/managed-npm-repair-reuse.md` and its paired design lifecycle.
- This plan and its four work-order result sections.

## Dependencies

Task 03. Existing private-recovery cases remain controls for legacy peers and unavailable storage.

## Risks

- An already-running agent can hide the missing cold-launch behavior.
- Old tests hard-code temporary-root cleanup; retain that assertion for legacy mode only.
- Public guidance must not promise reuse across a lost container filesystem or guarantee registry availability.

## Parallelism

`sequential`

## Inputs

- Plan: Presentation and mobile contract; E2E tests.
- Existing session/settings desktop and phone cache-recovery scenarios.
- `/docs-maintainer`, `/e2e`, `/mobile-parity`, and scoped web guidance before implementation.

## Results

Partially verified on 2026-11-08.

- Updated `docs/public/agents-and-profiles.md` with the reusable repair
  behavior, compatibility boundary, storage lifetime, and troubleshooting
  guidance. No rendered UI, copy, control, or locale change was required.
- Requirement 007 and its system design moved to `active` and `current`.
- Documentation checks passed: `python3 scripts/list-docs.py validate`
  (438 decisions, 1940 specifications), `python3 scripts/lint-spec-files.test.py`
  (36 tests), `python3 scripts/lint-spec-files.py --all`,
  `node --test scripts/validate-public-docs.test.mjs` (63 tests),
  `node scripts/validate-public-docs.mjs` (55 pages), the package coverage
  preflight (all four work orders covered), and `git diff --check`.
- Added the shared `reusable verified cache repair` session scenario and ran it:
  - `pnpm e2e:run --host --shards 1 --project chromium
    tests/session/managed-npm-cache-recovery.spec.ts --grep 'reusable verified cache repair'`
    passed (1 test).
  - `pnpm e2e:run --host --shards 1 --project mobile-chrome
    tests/session/mobile-managed-npm-cache-recovery.spec.ts --grep 'reusable verified cache repair'`
    passed (1 test).
  - The scenario stops the session, cold-resumes it, and asserts the installer
    count and registry requests do not increase, the recovery card stays absent,
    direct input is usable, and exactly one follow-up prompt is delivered.
- Settings/profile discovery reuse passed on desktop and mobile:
  the existing probe recovery scenario now refreshes the profile probe and
  asserts the registry request count does not increase.
  - `pnpm e2e:run --host --shards 1 --project chromium
    tests/settings/managed-npm-cache-recovery.spec.ts --grep 'reusable verified cache repair'`
    passed.
  - The combined desktop command covered both files and passed 2 tests; the
    combined mobile command passed 2 tests.
  - The session scenario restart, new execution identity, native conversation,
    and resume-token comparisons are all asserted in the shared scenario.
  - The session scenario also restarts the backend with execution-host storage
    intact, then asserts that the resumed cold launch has a new
    `agent_execution_id`, so an unchanged installer count cannot come from
    adopting the stopped process. It preserves the same session, and compares
    the downstream native conversation ID and resume token when present.
  - `pnpm e2e:run --host --shards 1 --project mobile-chrome
    tests/settings/mobile-managed-npm-cache-recovery.spec.ts` passed.
- Controls also passed: 7 desktop tests in `managed-runtime-startup-retry` and
  `profile-capability-discovery` (including saved catalog and unsaved draft
  refresh), desktop and mobile private-cache recovery (both runtime families),
  desktop and mobile profile probe recovery, and the Docker and SSH
  private-preparation specs.
- All desktop/phone session, settings, and control commands in the verification
  section passed. Same-storage Docker/SSH reuse is proven in Task 03.
