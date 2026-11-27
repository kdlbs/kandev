---
id: "03-retire-and-document"
title: "Retire obsolete composition and publish the contract"
status: done
wave: 3
depends_on:
  - "02-agent-composition"
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-001
  - REQ-AGENTS-AGENT-RICH-OUTPUT-002
acceptance_criteria:
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.17
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.20
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.1
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.4
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.6
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.9
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.10
system_design:
  - ../../specs/agents/system-design/agent-rich-output.md
---
# Task 03: Retire obsolete composition and publish

## Summary

Complete the replacement by removing unused custom composition code and
publishing the implemented contract. Retain explicitly named v1 compatibility,
native content components, and host authorization; verify the actual result
against the user's replacement goal before marking the package implemented.

## In scope

- Audit runtime imports and tests for the old `RichOutputBlockView` and any
  fallback JSX block dispatcher. Remove obsolete composition-only files or
  facades left during migration; keep version 1 decoding narrowly named and
  tested. Any code removal must preserve the real library path for both versions.
- Update Native rich output in `docs/public/automation-and-mcp.md`, including
  initial catalog, valid v2 examples, read-only boundaries, limits, and v1
  historical/cached-input compatibility. Do not expose implementation details
  as product instructions unless necessary for agent API authors.
- Update relevant scoped frontend AGENTS guidance and reconcile owning
  requirements/design with the final implementation. Keep historical completed
  plans as prior-delivery records; the new package is the replacement authority.
- Run docs-maintainer's impact search across public docs, root README, and
  screenshots. Record reference-page impact without unnecessary screenshots or
  a new documentation page.
- Record actual work-order validation results and final AC coverage. Promote
  the requirement to active, design to current, and plan to implemented only
  after every work order and referenced contract matches the delivered code.

## Out of scope

Removing historical replay, rewriting messages, replacing native content with
stock components, broad cleanup/review, committing, pushing, or opening a PR
without subsequent user authorization.

## Acceptance

1. Both input versions use json-render Renderer; no obsolete custom composition
   engine remains reachable, and v1 compatibility still passes its controls.
2. Public/scoped documentation describes the delivered catalog and authoring
   version accurately, with working examples that match schema conformance.
3. All work-order receipts and contract references are accurate; lifecycle
   promotion occurs only after implementation and required checks pass.

## Verification

From the repository root:

```bash
! rg -n 'RichOutputBlockView' apps/web
rg -n '@json-render/(core|react)' apps/web/lib/rich-output apps/web/components/task/chat/messages/kandev/rich-output
(cd apps/web && pnpm exec tsx scripts/generate-rich-output-catalog.ts --check)
(cd apps/web && pnpm exec vitest run lib/rich-output components/task/chat/messages/kandev/rich-output)
(cd apps/web && pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Import searches supplement, not replace, actual renderer tests and source
inspection. If removal changes backend behavior, rerun task 02's backend test
command. If removal changes rendered behavior, rerun its two targeted E2E
commands; otherwise do not repeat completed browser evidence without cause.
Run `.github/scripts/pr-docs.cjs`'s `validateCoverage` over the actual diff and
linked artifacts using the repository preflight procedure; inspect errors,
accepted references, and included work orders.

## Files likely touched

- Obsolete composition-only modules under
  `apps/web/components/task/chat/messages/kandev/rich-output/`, and their
  callers/tests if task 01 left migration facades
- `docs/public/automation-and-mcp.md`
- `apps/web/AGENTS.md`
- Owning requirement/design and this plan/work-order status/results
- Related guidance only when the actual impact search finds outdated claims

## Dependencies

Task 02, including its backend, frontend, and browser receipts.

## Risks

An import-only audit can miss a second rendering path. Inspect the real
`RichOutputRenderer` and compatibility adapter and preserve full-consumer tests.
Do not mark a design implemented merely because documentation gates pass.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/agent-rich-output.md)
- [Design](../../specs/agents/system-design/agent-rich-output.md)
- `/docs-maintainer`, specification lifecycle guide, and the actual implementation
  diff and prior work-order results

## Results

Completed 2026-10-08. Audited the runtime entry point, native registry,
normalizer, and imports. Removed the block dispatcher and obsolete public
parser/types; only the frozen historical decoder retains v1 blocks, MIME hints,
and positional snapshots. New native content uses catalog props, and backend
v1 authoring/result production is removed.

- Public docs updated: `docs/public/automation-and-mcp.md`, a reference section
  covering the catalog, v2 composition/CSV examples, limits, read-only controls,
  replay, stale callers, and catalog evolution. Both published v2 examples pass
  the actual frontend validator.
- Impact search covered `docs/public`, root README, screenshot catalog, scoped
  guidance, owning specs, and ADRs. No README or public screenshot change is
  needed for this agent API reference update. `apps/web/AGENTS.md` now records
  the catalog/generator/renderer/historical boundary.
- Requirements are active, design is current, and all work orders are done.
  Existing completed delivery plans remain historical records.
- Catalog drift, 155 frontend tests, typecheck, and task 02's backend/browser
  checks passed. Final frozen-lockfile install passed with pnpm 9.15.9.
- Specification catalog and lint passed; 36 specification-validator tests and
  62 public-documentation-validator tests passed; all 47 public pages validate.
- Documentation coverage preflight over the actual diff and all new files is
  covered with zero errors and all three work orders selected. All 31 criteria
  are linked through the delivery package. `git diff --check` passed.

Work remains uncommitted. No delegation, push, PR, or deployment was performed.
