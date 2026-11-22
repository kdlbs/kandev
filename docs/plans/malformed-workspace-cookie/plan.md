---
created: 2026-10-20
status: done
requirements:
  - REQ-AUTH-FIX-MULTI-INSTANCE-COOKIE-ISOLATION-001
system_design:
  - ../../specs/auth/system-design/fix-multi-instance-cookie-isolation.md
legacy_specs: []
---

# Implementation Plan: Malformed workspace cookie recovery

## Overview

Catch value-decoding failure in the shared browser cookie reader so bootstrap
continues through the existing validated selection paths. The user explicitly
authorized this small fix without a separate planning handoff on 2026-10-20.

## Scope

Own `route-bootstrap.ts`, focused cookie and settings-bootstrap regressions, and
the directly relevant auth requirement/design. Other decode calls, session
authentication, cookie naming, and rendered UI are outside scope.

## Technical approach

Current main `013facd0198f6a2b0192e32b5e694f4fa55c96c6` decodes the last
matching cookie without guarding `URIError`. `%E0%A4%A` and `%` reach that code
through the actual settings initial-state builder, aborting before hydration.
Return `null` when value decoding fails; preserve selection order and migration.
Remote main was checked using `git ls-remote`; open PR titles and relevant
routing PR files were inspected, with no competing cookie-decoding fix found.

## Tests

`route-bootstrap.cookie-recovery.test.ts` owns AC-AUTH-FIX-MULTI-INSTANCE-COOKIE-ISOLATION-001.9:
real cookie reads, malformed scoped/legacy values, UTF-8, empty/missing values,
last-match precedence, Office candidate fallback, settings hydration fallback,
validated migration, and one-time promotion. Existing routing/bootstrap suites
retain the scoped-name and workspace-validation contracts.

## E2E tests

No Playwright suite added or run. This is a shared browser bootstrap parser
with no rendered, pointer, layout, or viewport branches. The regression drives
real document cookies through the actual settings hydration builder and Office
candidate resolver. This shared behavior applies to desktop and phone routes;
no rendered desktop/mobile evidence is claimed.

## Work orders

- [x] [Task 01: Recover from malformed workspace cookies](task-01-cookie-recovery.md)

## Verification results

Causal RED: 9 of 13 focused regressions fail with `URIError` at the real decoder,
including the settings hydration builder; 4 compatibility controls pass.

- Dependencies: `pnpm install --frozen-lockfile` from `apps/` passed.
- Focused Vitest command in the work order: 4 files, 56 tests passed,
  including 15 cookie-recovery cases and real settings hydration.
- Changed-file ESLint: passed with zero warnings.
- `NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck`: passed,
  including rich-output catalog validation. The default 4 GB run exhausted
  Node's heap; an intermediate larger-heap run reported incomplete test
  fixtures, which were corrected before the final successful run.
- `python3 scripts/list-docs.py validate`: passed (394 decisions, 1681 specs).
- `python3 scripts/lint-spec-files.py --all`: passed.
- Local `.github/scripts/pr-docs.cjs` `validateCoverage` preflight: `covered`,
  no errors; work order, plan, AC and design references resolve.
- `git diff --check` and plan-path status inspection: passed; both new plan
  files are accounted for.
- No Playwright run: no rendered or viewport-dependent behavior changed.
- No public docs update: existing validated selection fallback is preserved;
  the internal requirement/design records explain decoding recovery.


## Risks

An undecodable last matching value must not revive an earlier duplicate value.
A malformed scoped value may be replaced only by an already validated legacy
selection. No architecture or security redesign is needed.
