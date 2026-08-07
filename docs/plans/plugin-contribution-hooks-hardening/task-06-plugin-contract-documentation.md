---
id: "06-plugin-contract-documentation"
title: "Plugin contract documentation"
status: completed
wave: 3
depends_on:
  - "01-authoritative-plugin-lifecycle"
  - "03-responsive-task-menu-context"
  - "04-bounded-mobile-plugin-panels"
plan: "plan.md"
spec: "../../specs/plugins/spec.md"
---

# Task 06: Plugin contract documentation

## Acceptance

- The frozen frontend contract and public authoring guide describe lifecycle-safe
  reloads, definitive revocation, grouped mobile panel discovery, and accurate
  responsive task-menu presentation.
- Documentation does not expose the host-internal lifecycle API as a plugin-facing
  method and does not claim changes to storage routes/schema/WS payloads.
- Spec, ADRs, `PLUGIN-API.md`, TypeScript comments, and public docs use consistent
  terminology.

## Verification

```bash
rg -n "registerTaskPanel|registerTaskMenuAction|host.storage|Panels" docs/public/plugins-authoring.md docs/plans/plugins/PLUGIN-API.md docs/specs/plugins/spec.md docs/decisions/2026-08-0*-plugin-*.md
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `docs/plans/plugins/PLUGIN-API.md`
- `docs/public/plugins-authoring.md`
- `docs/specs/plugins/spec.md` only for implementation-discovered wording corrections
- `docs/decisions/2026-08-01-plugin-task-panel-contributions.md`
- `docs/decisions/2026-08-04-plugin-contribution-lifecycle-authority.md`
- `apps/web/lib/plugins/types.ts` comments only if the public contract mirror needs
  clarification; no new plugin-facing methods

## Dependencies

Tasks 01, 03, and 04 so documentation describes the landed behavior precisely.

## Parallelism

`parallel-safe` with Task 05 after Tasks 01–04 finish; documentation and E2E files are
disjoint.

## Inputs

- Spec and ADRs named above.
- Plan: **Contract and documentation**.
- Public page type: reference/how-to guide for plugin authors.

## Risks

Do not document internal lifecycle transition methods as supported plugin API.

## Output contract

Report public/internal docs changed, validation results, contract drift checked, and
synchronize task/plan status/results.

## Results

- Updated `PLUGIN-API.md`, the public authoring guide, the frontend contract
  comments, and both plugin-contribution ADRs to describe lifecycle-safe reloads,
  definitive revocation, the grouped phone picker, and responsive menu context
  without exposing host-internal lifecycle methods or changing storage wire
  contracts.
- `rtk node --test scripts/validate-public-docs.test.mjs` — 58 tests passed.
- `rtk node scripts/validate-public-docs.mjs` — 41 published docs pages validated.
- `rtk pnpm run i18n:check && rtk pnpm run i18n:ratchet` — passed; the new Panels
  key is present in English and pseudo catalogs.
