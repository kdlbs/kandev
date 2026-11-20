---
id: "01-cookie-recovery"
title: "Recover from malformed workspace cookies"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AUTH-FIX-MULTI-INSTANCE-COOKIE-ISOLATION-001
acceptance_criteria:
  - AC-AUTH-FIX-MULTI-INSTANCE-COOKIE-ISOLATION-001.9
system_design:
  - ../../specs/auth/system-design/fix-multi-instance-cookie-isolation.md
---

# Task 01: Recover from malformed workspace cookies

## Summary

Guard the selected cookie value decode and prove bootstrap fallback plus
migration behavior with real reader calls.

## In scope

Cookie reader, focused regressions, directly relevant requirement/design.

## Out of scope

Other URI decoders, cookie naming, authentication, UI surfaces and controls.

## Acceptance

- Malformed values return null and settings/Office fallback continues.
- UTF-8, last-match precedence, scoped isolation and validated promotion remain.
- Leave legacy and unrelated cookies intact; never promote unknown IDs.

## Verification

Run from the repository root with Node 24 and pnpm on PATH:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/routing/route-bootstrap.cookie-recovery.test.ts lib/routing/route-bootstrap.test.ts lib/routing/route-bootstrap.team-access.test.ts src/settings-routes.bootstrap.test.ts)
(cd apps/web && pnpm exec eslint --max-warnings 0 lib/routing/route-bootstrap.ts lib/routing/route-bootstrap.cookie-recovery.test.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/malformed-workspace-cookie
```

## Files likely touched

- `apps/web/lib/routing/route-bootstrap.ts`
- `apps/web/lib/routing/route-bootstrap.cookie-recovery.test.ts`
- `docs/specs/auth/requirements/fix-multi-instance-cookie-isolation.md`
- `docs/specs/auth/system-design/fix-multi-instance-cookie-isolation.md`

## Dependencies

None.

## Risks

See [plan](plan.md#risks). No rendered UI changes; shared browser test evidence
covers the viewport-independent bootstrap path, without claiming Playwright runs.

## Parallelism

`sequential`

## Inputs

REQ-AUTH-FIX-MULTI-INSTANCE-COOKIE-ISOLATION-001 and its linked system design.

## Results

Causal RED: 9 of the initial 13 cases failed with URIError before production
changes, including actual settings bootstrap. Four compatibility controls passed.

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
