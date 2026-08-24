---
id: "08-dynamic-policy-settings-ui"
title: "Dynamic policy settings UI"
status: done
wave: 4
depends_on: ["02-versioned-policy-document", "03-durable-policy-evaluator"]
plan: "plan.md"
spec: "../../specs/platform/requirements/provider-error-recovery.md"
---

# Task 08: Dynamic policy settings UI

- **Acceptance:** Remove Add profile from the new dynamic profile route and add
  localized transient/hard policy editors for every candidate, including retry
  enablement, max retries, initial interval, reset-wait enablement, max wait,
  skip/stop outcome, derived schedule, and inline validation on desktop and
  phone.
- **Files likely touched:**
  `apps/web/app/settings/agents/[agentId]/agent-setup-parts.tsx`,
  `apps/web/components/settings/dynamic-agent-profile-editor.tsx`, new focused
  policy components/hooks under `apps/web/components/settings/`, profile types
  and normalization, locale catalogs, and component tests.
- **Dependencies:** Tasks 02 and 03.
- **Parallelism:** parallel-safe with Task 04. This task owns frontend settings
  and locale files; Task 04 owns backend conductor and orchestrator files.
- **Inputs:** Provider Error Recovery User interface and API; Dynamic Agent
  Routing Settings interaction; `/mobile-parity`; existing AgentProfilePicker,
  profile form fields, settings save coordinator, and create-mode draft flow.
- **Output contract:** Report create/edit mode distinction, desktop disclosure,
  phone composition and scroll owner, touch targets, validation and schedule
  copy, component split, files changed, exact commands/results, risks, and
  synchronized task/plan status.
- **Verification:** `cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- --run components/settings lib/api/domains/agent-profile-normalize.test.ts && pnpm --filter @kandev/web run typecheck && pnpm --filter @kandev/web run i18n:check && pnpm --filter @kandev/web run i18n:ratchet`
- **Risks:** The candidate row is already dense. Use progressive disclosure and
  one-column mobile layout; do not hide required explanations in hover-only
  help or create nested scroll containers.

## Results

Completed. Create mode now renders one draft without an enabled toggle or Add
profile action; edit mode retains the profile enablement control. Each
candidate has localized transient and hard policy editors for retry limits,
initial intervals, reset waits, exhausted outcomes, derived schedules, and
inline validation. The shared searchable agent-profile picker is reused for
candidate selection, with mobile-sized controls and a single page scroll
owner.

Verification:

- `pnpm test -- --run components/task/chat/dynamic-route-recovery.test.tsx components/settings/dynamic-agent-policy-editor.test.tsx lib/api/domains/agent-profile-normalize.test.ts components/settings/dynamic-agents-card.test.tsx` — 24 passed.
- `pnpm run lint` — 0 errors and 0 warnings.
- `pnpm run typecheck` — passed.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet` — passed.
