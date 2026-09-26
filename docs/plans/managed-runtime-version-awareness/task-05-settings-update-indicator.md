---
id: "05-settings-update-indicator"
title: "Show update awareness in Settings"
status: complete
wave: 4
depends_on: ["03-update-status-api"]
plan: "plan.md"
spec: "../../specs/agents/requirements/runtime-updates.md"
---

# Task 05: Show update awareness in Settings

## Acceptance

- Settings > Agents loads page-local batch status and shows a blue dot only for
  structural `update_available`; unknown status leaves the update control usable.
- The trigger exposes effective and latest versions in accessible copy, and
  opening it still fetches the authoritative live preview before mutation.
- Desktop keeps the existing dialog, mobile keeps the existing 44 px trigger
  and inset drawer, and **Use Kandev default** shares the same state/action logic.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- lib/api/domains/agent-update-api.test.ts hooks/domains/settings/use-agent-runtime-update-statuses.test.tsx components/settings/agent-runtime-update-control.test.tsx && pnpm --filter @kandev/web exec eslint app/settings/agents/page.tsx components/settings/installed-agent-card.tsx components/settings/agent-runtime-update-control.tsx hooks/domains/settings/use-agent-runtime-update-statuses.ts lib/api/domains/agent-update-api.ts && pnpm --filter @kandev/web i18n:check && cd web && pnpm run typecheck
```

## Files likely touched

- `apps/web/lib/api/domains/agent-update-api.ts`
- `apps/web/lib/api/domains/agent-update-api.test.ts`
- `apps/web/lib/types/http-agents.ts`
- `apps/web/hooks/domains/settings/use-agent-runtime-update-statuses.ts`
- `apps/web/hooks/domains/settings/use-agent-runtime-update-statuses.test.tsx`
- `apps/web/app/settings/agents/page.tsx`
- `apps/web/components/settings/installed-agent-card.tsx`
- `apps/web/components/settings/agent-runtime-update-control.tsx`
- `apps/web/components/settings/agent-runtime-update-control.test.tsx`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/agents.json`

## Dependencies

Task 03.

## Parallelism

Sequential. It consumes the final backend status and reset contracts.

## Inputs

- Spec: Desktop and mobile behavior, update status, failure modes
- Plan: Frontend and Mobile design contract
- Mobile exemplar: existing `AgentRuntimeUpdateControl`

## Output contract

Report rendered states, accessibility/mobile behavior, locale changes, exact
unit/lint/i18n/typecheck results, risks, and synchronized task/plan status.

## Results

Complete. Settings now loads page-local cached status, shows the blue dot only
for structural `update_available`, keeps unknown controls usable, refreshes
after Rescan and successful jobs, and shares the exact preview/mutation logic
between the desktop dialog and mobile drawer. Accessible labels include the
effective/latest versions, all new copy is localized in five catalogues, and
the 44 px mobile trigger remains intact.

Verification: focused frontend unit tests passed 49/49; full web lint passed;
`pnpm run typecheck` passed; `pnpm run i18n:check` passed with pseudo and all
four translated catalogues complete.

Follow-up review verification keeps the complete backend version projection in
the selector, prefers terminal effective versions, and clears a stale active
selection after a successful default reset. The focused frontend tests passed
32/32, the changed component lint passed, full web lint/typecheck/i18n passed,
and desktop/mobile E2E passed 15/15 and 4/4.
