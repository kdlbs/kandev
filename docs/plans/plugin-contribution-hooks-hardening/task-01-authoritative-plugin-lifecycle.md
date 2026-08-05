---
id: "01-authoritative-plugin-lifecycle"
title: "Authoritative plugin lifecycle"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/plugins/spec.md"
---

# Task 01: Authoritative plugin lifecycle

## Acceptance

- Plugin UI lifecycle exposes generation-fenced `loading`, `ready`, `failed`, and
  `removed` states without extending the plugin-facing registry contract.
- Open/saved task panels survive arbitrarily slow or failed initialization; only a
  ready generation missing the panel or an explicit removal closes it.
- Existing update, enable, disable, uninstall, modal, style, and registration cleanup
  behavior remains intact.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
pnpm --filter @kandev/web test -- lib/plugins/registry.test.ts lib/plugins/host.test.ts components/settings/plugins/use-plugin-actions.test.tsx components/task/use-close-revoked-plugin-panels.test.ts
```

The implementation must first add the delayed-load and terminal-state regression tests
and confirm they fail for the expected timer/lifecycle reason.

## Files likely touched

- `apps/web/lib/plugins/registry.ts`
- `apps/web/lib/plugins/registry.test.ts`
- `apps/web/lib/plugins/host.ts`
- `apps/web/lib/plugins/host.test.ts`
- `apps/web/lib/plugins/host-lifecycle.test.ts`
- `apps/web/components/settings/plugins/use-plugin-actions.ts`
- `apps/web/components/settings/plugins/use-plugin-actions.test.tsx`
- `apps/web/components/task/use-close-revoked-plugin-panels.ts`
- `apps/web/components/task/use-close-revoked-plugin-panels.test.ts`

## Dependencies

None.

## Parallelism

`parallel-safe` with Tasks 02, 03, and 07; files are disjoint. Task 04 depends on this
lifecycle authority.

## Inputs

- Spec: task-panel behavior, frontend failure modes, and slow reload scenarios.
- Plan: **Authoritative plugin UI lifecycle**.
- ADR-2026-08-04-plugin-contribution-lifecycle-authority.
- Existing generation fencing in `apps/web/lib/plugins/host.ts`.

## Risks

Timed-out initialization continues asynchronously; both its registry writes and its
lifecycle completion must be unable to supersede the current generation.

## Output contract

Report lifecycle transitions, files changed, the initial red-test failure, exact test
results, residual risks, and synchronize this task plus `plan.md` status/results.

## Results

- Red phase: the delayed-load, timeout-fencing, and panel-reconciliation assertions
  failed against the former fixed-delay/missing-registration behavior.
- Added host-internal `loading`/`ready`/`failed`/`removed` snapshots with generation
  fencing; reload and explicit removal now have distinct transitions. The panel hook
  reconciles only after authoritative lifecycle completion and has no elapsed timer.
- `rtk pnpm --filter @kandev/web test -- --run lib/plugins/registry.test.ts lib/plugins/host.test.ts lib/plugins/host-lifecycle.test.ts components/settings/plugins/use-plugin-actions.test.tsx components/task/use-close-revoked-plugin-panels.test.ts`
  — 5 files, 53 tests passed.
- Full frontend lint passed after moving lifecycle regressions into the dedicated
  `host-lifecycle.test.ts` file.
