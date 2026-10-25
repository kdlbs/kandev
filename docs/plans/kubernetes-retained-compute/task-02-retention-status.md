---
id: "02-retention-status"
title: "Show retained resource status"
status: done
wave: 2
depends_on:
  - "01-retention-projection"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-K8S-RETAINED-001
  - REQ-EXECUTORS-K8S-RETAINED-002
acceptance_criteria:
  - AC-EXECUTORS-K8S-RETAINED-001.1
  - AC-EXECUTORS-K8S-RETAINED-001.2
  - AC-EXECUTORS-K8S-RETAINED-001.3
  - AC-EXECUTORS-K8S-RETAINED-001.4
  - AC-EXECUTORS-K8S-RETAINED-001.5
  - AC-EXECUTORS-K8S-RETAINED-002.1
  - AC-EXECUTORS-K8S-RETAINED-002.2
  - AC-EXECUTORS-K8S-RETAINED-002.3
  - AC-EXECUTORS-K8S-RETAINED-002.4
  - AC-EXECUTORS-K8S-RETAINED-002.5
system_design:
  - ../../specs/executors/system-design/kubernetes-retained-compute.md
---

# Task 02: Show Retained Resource Status

## Summary

Expose the new projection in the current Active sessions table and phone cards.
Provide direct task navigation and visible guidance about resource retention.

## In scope

- Add optional frontend response fields and translated label/request helpers.
- Extend the existing component with separate session and Pod facts, main-container requests, and task navigation.
- Preserve existing polling, scope guards, and error behavior.
- Add component tests and extend desktop/mobile E2E for rendering, refresh, navigation, and geometry.
- Extend the real Kind managed-PVC scenario with profile-status assertions before Stop, after Stop, and after Resume.
- Update the reference/explanation sections in `docs/public/k8s.md` and relevant executor guidance.

## Out of scope

- New destructive buttons, global totals, saved filters, task-status schemas, or background polling.
- Changes to existing task route behavior or archive/delete authorization.

## Acceptance

- Desktop and phone show the same backend facts and navigate to the intended task without a mutation from the status controls.
- Refresh and real Stop/Resume evidence prove truthful retained classification, while unknown/error fields remain explicit.
- Translated guidance, touch targets, keyboard links, and long content pass focused rendered tests without document overflow.

## Verification

From `apps`, once for a fresh worktree:

```bash
rtk pnpm install --frozen-lockfile
```

From `apps/web`:

```bash
rtk pnpm exec vitest run components/settings/kubernetes-sessions-card.test.tsx lib/api/domains/kubernetes-api.test.ts hooks/domains/settings/use-kubernetes-settings.test.tsx
rtk pnpm run typecheck
rtk pnpm run i18n:check
rtk pnpm run i18n:ratchet
rtk pnpm e2e:run --project chromium tests/settings/kubernetes-executor.spec.ts
rtk pnpm e2e:run --project mobile-chrome tests/settings/mobile-kubernetes-executor.spec.ts
rtk env KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --project containers tests/kubernetes/kubernetes-executor.spec.ts -- --grep 'preserves a managed PVC'
```

From the repository root:

```bash
rtk node --test scripts/validate-public-docs.test.mjs
rtk node scripts/validate-public-docs.mjs
rtk git diff --check
```

Use TDD for new behavior. Run E2E commands sequentially with current builds.
The mobile scenario uses `.tap()`, asserts the card's task destination, and verifies
44px targets plus no document overflow. Capture one phone and desktop screenshot.
Generate Traditional Chinese values with `pnpm run i18n:zh-hant` during implementation.

## Files likely touched

- `apps/web/lib/types/http-kubernetes.ts`
- `apps/web/lib/api/domains/kubernetes-api.test.ts`
- `apps/web/components/settings/kubernetes-sessions-card.tsx`
- `apps/web/components/settings/kubernetes-sessions-card.test.tsx` (new)
- `apps/web/hooks/domains/settings/use-kubernetes-settings.test.tsx`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/executors.json`
- `apps/web/e2e/tests/settings/kubernetes-executor.spec.ts`
- `apps/web/e2e/tests/settings/mobile-kubernetes-executor.spec.ts`
- `apps/web/e2e/tests/kubernetes/kubernetes-executor.spec.ts`
- `docs/public/k8s.md`, `docs/public/executors.md`

## Dependencies

Task `01-retention-projection`. Real lifecycle evidence requires host Docker and
the current Kind fixture. UI response fixtures do not replace this evidence.

## Risks

- Old responses need unknown/unspecified fallbacks, not a duplicated frontend classifier.
- A whole-card phone link cannot contain nested interactive controls.
- Guidance must distinguish managed PVC deletion from operator-owned existing claims.
- Preserve changes from earlier packages in the shared Kind spec and public documentation.

## Parallelism

`sequential`

## Inputs

- [Retained-compute requirements](../../specs/executors/requirements/kubernetes-retained-compute.md)
- [Retained-compute design](../../specs/executors/system-design/kubernetes-retained-compute.md)
- `MobileSessionList` in `kubernetes-sessions-card.tsx`
- `apps/web/components/kanban-with-preview.tsx` for direct phone task navigation
- Existing desktop/mobile Kubernetes configuration E2E specs
- `.agents/skills/mobile-parity/references/kandev-mobile-ui-language.md`

## Results

Passed:

- `rtk pnpm exec vitest run components/settings/kubernetes-sessions-card.test.tsx lib/api/domains/kubernetes-api.test.ts hooks/domains/settings/use-kubernetes-settings.test.tsx`
  (24 tests, including distinct Pod phase and main-container state coverage on
  desktop and mobile)
- `rtk pnpm run typecheck`
- `rtk pnpm run i18n:check`
- `rtk pnpm run i18n:ratchet`
- `rtk pnpm e2e:run --host --no-build --project chromium tests/settings/kubernetes-executor.spec.ts`
  (4 tests passed)
- `rtk pnpm e2e:run --host --no-build --project mobile-chrome tests/settings/mobile-kubernetes-executor.spec.ts`
  (3 tests passed)
- `rtk env KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --no-build --project containers tests/kubernetes/kubernetes-executor.spec.ts -- --grep 'preserves a managed PVC'`
  (1 test passed in 53 seconds; retained and active projections matched
  Stop/Resume, and terminal cleanup removed the managed resources)
- `rtk node --test scripts/validate-public-docs.test.mjs`
- `rtk node scripts/validate-public-docs.mjs`
- `rtk git diff --check`

Compact-row follow-up passed:

- The focused component, API, and hook suite passed 32 tests.
- The desktop E2E suite passed 4 tests. The session row remained at most 96 px
  high, and the Created cell remained inside the table.
- The phone E2E suite passed 3 tests. The status group remained at most 80 px
  high with no document overflow.
- Production-build screenshots confirmed the compact desktop row and phone card.
