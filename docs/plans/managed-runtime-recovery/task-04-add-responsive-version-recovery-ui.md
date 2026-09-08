---
id: "04-add-responsive-version-recovery-ui"
title: "Add responsive version recovery UI"
status: done
wave: 4
depends_on: ["03-activate-validated-runtime-versions"]
plan: "plan.md"
spec: "../../specs/agents/requirements/runtime-updates.md"
---

# Task 04: Add responsive version recovery UI

## Acceptance

- The API client previews a selected target and sends it explicitly in POST.
- The shared dialog state defaults to backend latest, refreshes the trusted
  preview when selection changes, ignores stale responses, and preserves the
  selected target through job progress and retry.
- Structural operation state renders **Update runtime**, **Roll back runtime**,
  **Repair runtime**, or disabled **Up to date** in every locale.
- Desktop dialog and phone drawer share version selection and business logic;
  the picker is keyboard/touch accessible with visible latest/active markers.
- Registry, candidate, and activation failures allow retry or another version
  selection without exposing package/command input.

## TDD sequence

1. Add failing API tests for encoded preview query and JSON POST body.
2. Add failing hook tests for default selection, target changes, stale preview
   cancellation, active-job idempotency, and exact-target retry.
3. Add failing pure-state tests for operation labels and approval gates without
   translated-string comparison.
4. Implement the shared selector/body/footer and all localized copy.
5. Run focused tests, typecheck, lint, i18n check, and i18n ratchet.

## Files likely touched

- `apps/web/lib/api/domains/agent-update-api.ts`
- `apps/web/lib/api/domains/agent-update-api.test.ts`
- `apps/web/lib/agent-runtime-update.ts`
- `apps/web/lib/agent-runtime-update.test.ts`
- `apps/web/components/settings/use-agent-update-dialog-state.ts`
- `apps/web/components/settings/use-agent-update-dialog-state.test.ts`
- `apps/web/components/settings/agent-runtime-update-control.tsx`
- `apps/web/src/locales/{en,pt-pt,zh-cn,pseudo}/agents.json`

## Mobile requirements

- Keep the existing bottom drawer and introduce no nested drawer.
- Keep all interactive controls at least 44 px high.
- Keep the shared body as the only vertical scroll owner and the footer inside
  the safe-area inset.
- Avoid browser-chrome collisions and horizontal page overflow for long
  versions, commands, and output.

## Verification

Fresh-worktree bootstrap, when needed:

```bash
cd apps && pnpm install --frozen-lockfile
```

```bash
cd apps && pnpm --filter @kandev/web test -- --run lib/api/domains/agent-update-api.test.ts lib/agent-runtime-update.test.ts components/settings/use-agent-update-dialog-state.test.ts
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm run i18n:check
cd apps/web && pnpm run i18n:ratchet
```

## Risks

- Do not classify versions in TypeScript; render backend operation state.
- Do not compare localized copy to decide state.
- Avoid refetch loops and stale selection overwrites when targets change fast.

## Output contract

Record RED/GREEN evidence, responsive behavior, checks, and risks in Results.
Update this task and `plan.md` status.

## Results

RED covered encoded target queries, exact POST bodies, stale preview response
handling, exact-target approval, operation labels, and localized missing-version
gates. GREEN verification:

- `rtk pnpm --filter @kandev/web test -- --run lib/api/domains/agent-update-api.test.ts lib/agent-runtime-update.test.ts components/settings/use-agent-update-dialog-state.test.ts hooks/domains/settings/use-agent-runtime-updates.test.tsx` — 31 tests passed across 4 files.
- `rtk pnpm run typecheck` — passed.
- `rtk pnpm --filter @kandev/web lint` — passed.
- `rtk pnpm run i18n:check` — passed; existing 117 real-locale parity warnings remain advisory.
- `rtk pnpm run i18n:ratchet` — passed with 0 added and 8 modified files clean.

The desktop dialog and mobile drawer share the selector and state machine,
submit only trusted exact targets, derive action copy from backend operation
state, and retain 44 px mobile controls with one scroll owner.
