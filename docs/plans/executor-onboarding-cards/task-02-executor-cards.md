---
id: "02-executor-cards"
title: "Present supported executors in onboarding"
status: done
wave: 2
depends_on:
  - "01-phone-availability"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-ONBOARDING-001
acceptance_criteria:
  - AC-EXECUTORS-ONBOARDING-001.1
  - AC-EXECUTORS-ONBOARDING-001.2
  - AC-EXECUTORS-ONBOARDING-001.3
  - AC-EXECUTORS-ONBOARDING-001.4
  - AC-EXECUTORS-ONBOARDING-001.5
  - AC-EXECUTORS-ONBOARDING-001.6
  - AC-EXECUTORS-ONBOARDING-001.7
  - AC-EXECUTORS-ONBOARDING-001.8
system_design:
  - ../../specs/executors/system-design/first-run-discovery.md
---

# Task 02: Present supported executors in onboarding

## Summary

Replace the four-row executor tour content with six accurate, localized cards on supported viewports. Keep tour actions intact, update the public first-run description, and prove that users can read every option and continue.

## In scope

- Add the two missing operational types and accurate Worktree, Local, Docker, and remote prerequisites.
- Show a two-column card grid in a bounded dialog body on supported viewports.
- Add guide and Settings direction without card selection or configuration mutation.
- Add focused component and desktop Playwright coverage, translations, and public docs.

## Out of scope

- Runtime, API, data model, executor-profile, and task-default changes.
- Remote Docker, test-only executor types, other tour-step content, and new onboarding screens.

## Acceptance

1. The executor step shows only the six operational choices, accurate prerequisite and trust text, a profile explanation, and the guide link. Cards have no selection state.
2. The desktop layout matches UI-01, with all cards and the guide link reachable through the dialog's scrollable body. The entire tour stays hidden on phones under Task 01.
3. Existing Back, Next, Skip, dirty agent-profile save, and browser-local completion behavior remain intact. New copy passes locale checks and public docs describe the result.

## ASCII UI preview

UI-01 in the [plan](plan.md#ui-01-executors-step-desktop) is the executor-step composition contract. It maps to `AC-EXECUTORS-ONBOARDING-001.1` through `.8`.

```text
UI-01 DESKTOP: two information-card columns
+----------------------------+  +----------------------------+
| Worktree   RECOMMENDED      |  | Local      BUILT IN        |
| Separate Git checkout.     |  | Selected folder.           |
+----------------------------+  +----------------------------+
| Docker     DAEMON NEEDED    |  | SSH        HOST SETUP       |
+----------------------------+  +----------------------------+
| Sprites    PROVIDER SETUP   |  | Kubernetes ADMIN SETUP     |
+----------------------------+  +----------------------------+
                   [View executor guide]
       [Skip]                         [Back] [Next]

```

The drawing fixes hierarchy and scroll ownership; spacing and exact line wraps are illustrative.

## Verification

First write the component and E2E assertions and observe their failures. Then implement and run:

```bash
cd apps/web
pnpm exec vitest run components/onboarding-dialog.test.tsx
pnpm run typecheck
pnpm run i18n:zh-hant
pnpm run i18n:pseudo
pnpm run i18n:check
pnpm run i18n:ratchet
pnpm exec eslint components/onboarding-dialog.tsx components/onboarding-dialog.test.tsx e2e/tests/office/onboarding-executors.spec.ts
pnpm e2e:run --project chromium tests/office/onboarding-executors.spec.ts
```

For the public guide and design records, run from the repository root:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

If this is a fresh worktree, run `cd apps && pnpm install --frozen-lockfile` before the first pnpm command.

## Files likely touched

- `apps/web/components/onboarding-dialog.tsx`
- `apps/web/components/onboarding-dialog.test.tsx`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-tw,zh-hk,ja,pseudo}/common.json`
- `apps/web/e2e/tests/office/onboarding-executors.spec.ts`
- `docs/public/use-kandev.md`

Consult the Settings hub and its executor-type registry for catalog parity; move no settings controls as part of this work order.

## Dependencies

Task 01 establishes phone availability. Read the executor requirement, design, and dialog-content containment contract first.

## Risks

- The shared footer affects every tour step. Keep its transition and save behavior unchanged.
- A fixed-height card grid can hide Kubernetes or the Next action on a supported viewport.
- Remote Docker remains in an old create-route map. Do not infer support from that map.

## Parallelism

`sequential`

## Inputs

- [Executor onboarding requirement](../../specs/executors/requirements/first-run-discovery.md)
- [Executor onboarding design](../../specs/executors/system-design/first-run-discovery.md)
- [Dialog-content containment requirement](../../specs/ui/requirements/dialog-content-containment.md)
- [Executor guide](../../public/executors.md)

## Results

Implemented six static, localized executor cards in Worktree-first order. The cards match the operational Settings choices and exclude Remote Docker and test-only types. Added setup and trust-boundary notes, the executor/profile explanation, Settings direction, and the executor-guide link. The Local card says work runs on the Kandev host. The executor step uses a bounded dialog with a scrollable body, keeping its heading, progress, and controls visible. Updated the public Get Started guide. Save actions now allow only one in-flight request across Next and Get Started, disable footer actions while saving, and show a localized error while keeping the current step open when an unexpected save fails. Updated both requirements to active and both designs to current. After generating the Traditional Chinese onboarding values, restored unrelated generated changes so the `zh-tw` and `zh-hk` diffs contain only `common.json`.

Verification passed:

- `pnpm exec vitest run components/onboarding-dialog.test.tsx app/page-client.test.tsx` (32 tests, including repeated Next/Get Started clicks and unexpected save errors)
- `pnpm run typecheck` and `pnpm run build:e2e`
- `pnpm run i18n:zh-hant`, `pnpm run i18n:pseudo`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet`
- Targeted ESLint and Prettier checks
- Desktop executor browser test (1 test) and mobile onboarding browser tests (2 tests)
- Public-doc tests (62), public-doc validation (47 pages), specification validation (305 decisions and 1,155 specs), specification lint, and `git diff --check`
