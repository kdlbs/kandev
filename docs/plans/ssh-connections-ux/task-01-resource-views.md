---
id: "01-resource-views"
title: "Build focused SSH collection, overview and edit views"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-CONNECTIONS-001
  - REQ-EXECUTORS-SSH-CONNECTIONS-002
  - REQ-EXECUTORS-SSH-CONNECTIONS-004
  - REQ-EXECUTORS-SSH-CONNECTIONS-005
acceptance_criteria:
  - AC-EXECUTORS-SSH-CONNECTIONS-001.6
  - AC-EXECUTORS-SSH-CONNECTIONS-002.3
  - AC-EXECUTORS-SSH-CONNECTIONS-002.6
  - AC-EXECUTORS-SSH-CONNECTIONS-004.3
  - AC-EXECUTORS-SSH-CONNECTIONS-004.4
  - AC-EXECUTORS-SSH-CONNECTIONS-004.8
  - AC-EXECUTORS-SSH-CONNECTIONS-004.9
  - AC-EXECUTORS-SSH-CONNECTIONS-004.12
  - AC-EXECUTORS-SSH-CONNECTIONS-005.6
system_design:
  - ../../specs/executors/system-design/ssh-connection-management-ui.md
---

# Task 01: Build focused SSH resource views

## Summary

Replace the narrow name-only list and always-editable card with a full-width
collection and focused saved overview. Add explicit metadata edit and align
identity presentation with the same settings hierarchy.

## In scope

- Shared header tabs, semantic full-row links, name/endpoint/status metadata,
  same-name discriminators, empty/loading/retry/unavailable states.
- Validated section/mode route state, old bookmark compatibility, one scroll
  owner, Back/list-focus restoration and coordinated metadata Edit/Save/Reset.
- Read-only connection and identity summaries, public-key disclosure/copy,
  visible dependency links and overflow deletion with existing confirmation.
- Move current access/diagnostic operations into their explicit sections as an
  intermediate usable state. Task 02 refines their internal progression.
- All language catalogs and pseudo for this slice; affected unit/browser helpers.

## Out of scope

New data APIs, record deduplication/deletion during migration, enrollment state
redesign, global shared-component restyling, and executor hub action placement.

## Acceptance

1. UI-01/UI-02 collections and saved overviews work through canonical/old URLs,
   keyboard and phone row activation. Ready overview has no editable metadata,
   bootstrap form, raw public-key block or inline destructive action.
2. Edit uses one existing save contributor, preserves dirty/late-edit behavior,
   and invalidates dependent results through current route data. Deletion keeps
   confirmations/reference guards; identity dependencies remain actionable.
3. Desktop/phone/coarse geometry, focus, long values and pseudo-locale checks
   pass. No navigation or section choice starts SSH operations.

## UI preview

UI-01 and UI-02 excerpt; see [full previews](plan.md#ui-previews).
The task's structured-preview rule replaces ASCII drawings.

| View | Desktop | Phone |
| --- | --- | --- |
| Collection | Header tabs + New; full-width resource rows | Tappable rows with context below name/status |
| Overview | Back; name/status; Edit/menu; grouped summary; Test action | Focused summary then reachable Test action |
| Edit | Expanded endpoint/identity groups; shared floating Save | Single column, picker drawers, safe-area Save |
| Identity | Public metadata and dependency links; Show/Copy public key | Same focused page with wrapping values |

The full-width collection, explicit edit, one scroll owner and hidden routine
bootstrap form are structural. Labels are illustrative. Relevant ACs are listed
in frontmatter.

## Verification

Use `/tdd`: write failing state, route and edit tests before implementation.
`connection-presentation.test.ts`, `settings.test.tsx`, and the two layout specs
below are new planned files; no-tests-found is a failure. Adapt affected existing
helpers to explicit overview/edit navigation rather than removing assertions.
Run from repo root. Browser projects run sequentially.

```bash
(cd apps/web && pnpm exec vitest run lib/settings/ssh-connection-settings-routes.test.ts components/settings/ssh-connections hooks/domains/settings/use-ssh-connections.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/settings/ssh-connections lib/settings/ssh-connection-settings-routes.ts lib/settings/ssh-connection-settings-routes.test.ts e2e/helpers/ssh-identities.ts e2e/helpers/ssh-identity-dependencies.ts e2e/tests/settings/ssh-management-layout.spec.ts e2e/tests/settings/mobile-ssh-management-layout.spec.ts)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
pnpm --dir apps/web e2e:run --host --project chromium tests/settings/ssh-management-layout.spec.ts tests/settings/ssh-connections.spec.ts tests/settings/ssh-identity-dependencies.spec.ts
pnpm --dir apps/web e2e:run --host --project mobile-chrome tests/settings/mobile-ssh-management-layout.spec.ts tests/settings/mobile-ssh-connections.spec.ts tests/settings/mobile-ssh-identity-dependencies.spec.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Layout specs cover 390/767/768/1280px and a coarse 820px context, ready/incomplete
rows, duplicate names, empty/retry/removed states, Back/Forward, focus, actual
control dimensions, zero document overflow, light/dark, and pseudo locale.
Use request observers to prove no probe/mutation on view transitions. Stubbed
ready metadata is UI evidence only. Preserve direct/bastion compatibility tests.

## Files likely touched

Relative to `apps/web/`:

- `components/settings/ssh-connections/{settings,connection-details,identity-details,identity-form,identity-dependencies,use-manager-navigation}.tsx` (navigation hook is `.ts`).
- Proposed local `connection-collection.tsx`, `connection-overview.tsx`,
  `connection-presentation.ts`, `connection-presentation.test.ts`, `settings.test.tsx`.
- `lib/settings/ssh-connection-settings-routes.ts` and `.test.ts`.
- Existing connection/identity component tests and settings browser helpers/specs
  listed above; proposed desktop/phone layout specs.
- `src/locales/{en,pseudo,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/settings.json`.

## Dependencies

None. Read the package and preserve baseline `b3517cdd869`.

## Risks

Stale status derived from name/revision alone; lost navigation context; hidden
warnings; confusing identical records; save-provider ownership changes.

## Parallelism

`sequential`

## Inputs

- [Owning requirements](../../specs/executors/requirements/ssh-connection-manager.md).
- [Interaction design](../../specs/executors/system-design/ssh-connection-management-ui.md):
  navigation, collections, saved overview, identity, and mobile sections.
- Current settings header/group/tabs/save primitives and executor profile rows.

## Results

Completed locally on 2026-10-26. Full-width collections, focused overviews,
explicit edits, shared save ownership, public-key disclosures, dependency links,
overflow deletion and validated navigation are implemented.

RED: URL allowlist, current-stamp state/disambiguation, and navigation regressions
failed on old behavior. The browser overview regression reproduced Host input
count 1 instead of 0 under the old composition. An initial browser build was
invalidated by locale generation; it is not a product test result. The first
new composition run found a status badge inside the resource heading; moving
status to the description fixed the accessible name.

GREEN: 46 unit cases in 10 files; typecheck; scoped ESLint; localization and
new-copy ratchet; catalog/spec/whitespace checks. All 15 distinct desktop and
9 phone browser cases passed across the full run and targeted retry. The retry
fixed test navigation to the explicit Access tab, distinguished keyboard from
actual touch capability, and selected collection rows by their resource IDs.
No assertions were removed. A final read-only row spacing/name-duplication
polish passed fresh desktop and phone rendered checks (one case each).

Receipts: /tmp/ssh-ux-state-green.log, /tmp/ssh-ux-navigation-red.log,
/tmp/ssh-ux-unit-resource-final.log, /tmp/ssh-ux-resource-{lint,type}-final.log,
/tmp/ssh-ux-i18n-resource.log, /tmp/ssh-ux-i18n-ratchet-resource.log,
/tmp/ssh-ux-resource-desktop.log, /tmp/ssh-ux-resource-desktop-retry.log,
/tmp/ssh-ux-resource-phone.log and /tmp/ssh-ux-overview-polish-{desktop,phone}.log.
Desktop and phone screenshots were inspected. Access still uses the existing
panels within its own tab; Task 02 owns their focused interaction. No running
instance, live host, publication or feature-default change occurred.
