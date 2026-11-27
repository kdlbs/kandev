---
id: "01-linked-task-identities"
title: "Match current project identities in work-item detail"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001
acceptance_criteria:
  - AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.12
  - AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.13
  - AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.14
system_design:
  - ../../specs/integrations/system-design/azure-devops-integration-02.md
---

# Task 01: Match current project identities in work-item detail

## Summary

Show normal project-name associations in detail opened with a selected project
GUID, while preserving ID associations and workspace/project/work-item
isolation. First add independent tests through the real hook; then change its
display predicate with no backend or layout change.

## In scope

- The shared hook's association selection and targeted consumer regression
  tests, with typed coherent first-party work-item and association data.
- Nonblank exact selected-ID/current-item-name alternatives, missing-name
  fallback, coinciding aliases, multiple links, and mixed negative controls.
- Update this order and manifest with actual results after implementation.

## Out of scope

Backend association validation, canonicalization/migration, provider mutation,
auth, pagination, new API, cache/stale-read/save companions, layout, new copy,
private ROOT fixtures/harness, unrelated Playwright suites, native delegation,
new sessions, model switching, and merge.

## Acceptance

1. The actual hook returns the expected nonempty name-valued linked row for a
   GUID-selected project, and existing GUID rows remain visible (`.12`).
2. All and only matching workspace/project/item rows survive; missing/blank
   aliases cannot match empty values, coinciding aliases do not duplicate a
   row, and several matching tasks remain present (`.12` through `.14`).
3. The scoped checks below pass with actual RED/GREEN evidence. Production
   changes remain confined to the hook and shared desktop/phone behavior stays
   data-only. No new interaction, layout, or copy is introduced.

## UI outcome and mobile parity

Use the existing detail surface's Linked Kandev tasks section on desktop and
phone. The changed data is described in the
[plan](plan.md#ui-outcome-and-mobile-parity); no geometry preview is needed.
The shared-hook tests cover the mobile-parity data-only exception and do not
claim rendered phone verification.

## TDD sequence

1. After ROOT's later explicit implementation interrupt, mark this order
   `in_progress`. Recheck the admitted source/base and dependency presence;
   hold the sole local-heavy lease before running package commands.
2. Extend the existing detail test file using `renderHook`, the real hook, and
   endpoint mocks only. Keep the comment merge test. Author independent
   first-party-shaped fixtures; never read/copy/replay ROOT private sources.
   Use the six test names in the [plan](plan.md#tests). Assert complete linked
   rows after loading finishes, including a nonempty causal expectation.
   Assert workspace request scope and put foreign rows beside valid rows.
3. Run the focused test command RED against unchanged production. Require the
   name/GUID case to fail for the expected missing-row reason; infrastructure
   failures are not valid RED. Record actual exit and test results.
4. Modify only `loadLinkedTasks`: require row workspace and item identity, plus
   exact selected-ID or usable `initialItem.project`. Reject blank aliases
   without normalizing comparison values. Filter once to preserve all matching
   associations and avoid alias-induced duplicates. Preserve existing effects,
   cache behavior, and API calls.
5. Run GREEN and the other scoped checks once. Record real counts, exits,
   native handles/joins and owned process absence. Update Results and manifest;
   continue normal delivery under the plan's ROOT gates.

## Verification

From repository root, with canonical tool binaries on PATH (Node 24.21.0 and
pnpm 9.15.9 from `/home/jcfs/.local/share/mise/installs`), run each command from
its explicit directory. If `apps/node_modules` is missing, first run
`(cd apps && pnpm install --frozen-lockfile)` ONCE under the heavy lease.
Use a fresh task-owned `KANDEV_RUN_QUIET_DIR` before every new `run-quiet`
wrapper. Do not launch these package checks during design.

```bash
# Run once for RED and once after the implementation for GREEN.
(cd apps/web && pnpm exec vitest run hooks/domains/azure-devops/use-azure-devops-work-item-detail.test.ts --maxWorkers=1)
# GREEN-only checks.
(cd apps/web && pnpm exec eslint --max-warnings 0 hooks/domains/azure-devops/use-azure-devops-work-item-detail.ts hooks/domains/azure-devops/use-azure-devops-work-item-detail.test.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
/home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node /tmp/kandev-azure-linked-task-identities-8c223f1b-MD1mkD0i/docs-coverage-actual.cjs
git diff --check
git status --short -- docs/plans/azure-linked-task-identities
```

Also run the repository documentation-coverage `validateCoverage` export from
`.github/scripts/pr-docs.cjs`, using actual changed paths and the referenced
four Markdown files as `fileContents`. The exact invocation is in the block
above. During design the four actual docs-only paths may report `EXEMPT`
with `errors: []`. After implementation the six actual changed paths must
report `COVERED`, `errors: []`, and this single work order. Never add a
prospective path to an actual changed-path report. An optional prospective
traceability rehearsal is separate evidence, not a required covered result.
Store the actual preflight receipt and invocation with task-owned logs.

The existing test file is assigned to the `browser` project in
`vitest-reviewed-projects.json`. Its `happy-dom` environment, `vitest.setup.ts`,
`NODE_ENV=test` pin and `React.act` preflight support actual `renderHook` tests;
no registry or configuration edit is planned.

No Go source is owned and no Go tests/lint or full frontend/E2E suite is
required locally. If a check uncovers an unrelated baseline issue, preserve
evidence and report it; do not broaden this repair silently.

## Files likely touched

- `apps/web/hooks/domains/azure-devops/use-azure-devops-work-item-detail.ts`
- `apps/web/hooks/domains/azure-devops/use-azure-devops-work-item-detail.test.ts`
- `docs/specs/integrations/requirements/azure-devops-integration.md`
- `docs/specs/integrations/system-design/azure-devops-integration-02.md`
- `docs/plans/azure-linked-task-identities/plan.md`
- `docs/plans/azure-linked-task-identities/task-01-linked-task-identities.md`

## Dependencies

None. The reviewed design package and ROOT's later implementation interrupt
are the execution prerequisite. Existing broad Azure implementation orders
remain historical records and are not rerun by this order.

## Inputs

- `REQ-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001` and its `.12` through `.14`
  criteria; paired design's Task/detail association identity section.
- Current hook, typed Azure models/API, launcher and board selection flow.
- Nearby `use-azure-devops-board.test.tsx` for actual-hook testing style.
- Receipt-only qualification and admission paths in the manifest.

## Risks

Matching arbitrary normalized strings could cross project boundaries; exact
current-item aliases and mixed exclusions must prove the constraint. Pure
helper tests cannot establish the real hook consumes the name alias correctly.
Do not imply this display change fixes backend GUID association validation.

## Parallelism

`sequential`, SAME primary. No native delegates or extra platform sessions.

## Results

Done. The only production edit is the `loadLinkedTasks` predicate. All three
implementation acceptance conditions passed with actual-hook tests and no
backend/API/schema/cache/lifecycle/layout change.

- Frozen install ONCE: passed; native 53100 joined 0.
- Focused Vitest RED: native 25595 joined 1, 2 failed / 7 passed. The causal
  name-valued first-party link was expected nonempty and returned `[]` while
  production was unchanged. The mixed-row test also failed as expected.
- Same focused Vitest GREEN: native 6633 joined 0, all 9 tests passed,
  including the retained comment-merging test.
- Scoped ESLint: native 40662 joined 0.
- Typecheck: native 12274 joined 134 after default 4 GB Node heap exhaustion;
  retry with `NODE_OPTIONS=--max-old-space-size=8192`, native 62756 joined 0.
  This was a local resource correction, with no config/source change.
- Staged i18n ratchet: native 19132 joined 0. All six actual paths were staged.
- Catalog validation (386 decisions / 1,633 specs), all-spec lint and actual
  six-path documentation coverage: native 59079 joined 0. Coverage was
  `COVERED`, `errors: []`, with this one order. Whitespace/status checks passed.

All owned command groups were absent at join. Exact argv, exit/join receipts,
unique `KANDEV_RUN_QUIET_DIR` values and raw logs are in
`/tmp/kandev-azure-linked-task-identities-8c223f1b-MD1mkD0i/`, using the
`install`, `red`, `green`, `lint`, `typecheck`, `typecheck-8gb`, and `ratchet`
`*-latest.json` pointers. Normal active commit hooks and same-turn hosted
observation follow under ROOT's separate delivery/merge gates.

Design checks are historical: `design-final-checks.json` records actual four
paths as `EXEMPT`, with no errors. The earlier prospective rehearsal is not an
actual changed-path report. The spec-linter self-tests passed 36 tests during
design only; they were removed from required implementation checks and not
replayed. No browser/mobile layout or live-provider verification is claimed;
the shared-hook data-only mobile exception applies. Public docs already describe
linked-task visibility, so no public wording change is needed.
