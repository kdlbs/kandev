---
id: "08-assistant-ui"
title: "First-class assistant interface"
status: done
wave: 6
depends_on: ["02-objectives-routing","03-memory-context","04-capability-inventory","07-input-resolution"]
plan: "plan.md"
requirements:
  - REQ-ORCHESTRATION-ASSISTANT-007
acceptance_criteria:
  - AC-ORCHESTRATION-ASSISTANT-007.1
  - AC-ORCHESTRATION-ASSISTANT-007.2
  - AC-ORCHESTRATION-ASSISTANT-007.3
  - AC-ORCHESTRATION-ASSISTANT-007.4
system_design:
  - ../../specs/orchestration/system-design/personal-assistant.md
---

# Task 08: First-class assistant interface

## Inputs

Read the [requirements](../../specs/orchestration/requirements/personal-assistant.md) and [design](../../specs/orchestration/system-design/personal-assistant.md); legacy scenarios S01, S06, S07, S09, S11, S12, S19, S20, and [plan](plan.md), Frontend. Read applicable AGENTS.md and implementation skills before editing. The [baseline experiments](experiments.md) are continuation evidence, not completed implementation.

## Acceptance

1. App-level Assistant navigation resumes the selected conversation on desktop/mobile with no Office setup and no duplicate tasks or workflows.
2. Goals, working/delivery status, actionable native input and memory provenance/forget controls are usable without opening board tasks.
3. Stable client message IDs, cursor/revision refresh and reconnect handle retries and task-tab resolution without duplicates or stale approvals.

## Likely files

- apps/web/src/spa-routes.tsx; components/app-sidebar and mobile app navigation
- apps/web/app/assistant/assistant-page.tsx, attention-card.tsx, memory-panel.tsx and tests (new)
- apps/web/app/settings/orchestration/conversation-pane.tsx
- apps/web/lib/api/domains/assistant-api.ts (new); orchestration-conversation-api.ts and tests
- apps/web/hooks/domains/orchestration/use-assistant.ts, use-assistant.test.ts (new)
- apps/web/components/task/chat/messages/permission-request-message.tsx, permission-action-row.tsx, clarification-request-message.tsx (extract transport interfaces only as needed)
- apps/web/src/locales/*/orchestration.json

## Implementation sequence

Read web AGENTS, frontend and mobile-parity skills before UI edits. Reuse shared chat and native input presentations with injectable transports; add progressively disclosed goal/activity/context panels. Keep semantic keyboard/focus behavior, small-screen scrolling and non-color status labels. Add stable message IDs to optimistic-send/retry state; invalidated cards refetch authorized current DTOs.

## Verification

Run each parenthesized command from the repository root. Use the repository Go/Node/pnpm toolchains. Scoped Go tests are intentional: the available make test target runs the entire backend. New test filters must select the named new tests; a no-tests-to-run result does not satisfy acceptance.

```sh
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web test app/assistant/assistant-page.test.tsx app/assistant/attention-card.test.tsx app/assistant/memory-panel.test.tsx hooks/domains/orchestration/use-assistant.test.ts lib/api/domains/orchestration-conversation-api.test.ts)
(cd apps/web && pnpm run typecheck)
```

## Dependencies and risks

Dependencies: `02-objectives-routing`, `03-memory-context`, `04-capability-inventory`, `07-input-resolution`. Execute in the primary session unless the user explicitly authorizes subagents.

Shared chat components serve Office and tasks too; preserve their transport defaults and avoid leaking their property UI or unrestricted API calls into Assistant. All supported locales must render without missing keys.

## Output

An accessible central assistant surface over the tested native contracts.

## Detailed implementation checklist

1. Add `/assistant` and desktop/mobile navigation gated by the effective assistant
   flag. Render loading/unconfigured/unavailable/empty/active states. A disabled
   direct URL must not fetch private history or create a default binding.
2. Reuse the native OrchestratorConversationPane/TaskChat transport. Implement an
   owner-scoped assistant API/hook with stable client-message IDs retained across
   retry and reconnect, abort/generation guards on owner/binding/workspace changes,
   and revision-only invalidation followed by authorized refetch.
3. Build progressively disclosed objectives/evidence, attention, capabilities,
   activity and memory panels. Distinguish working, delivered, waiting, review,
   interrupted and unknown; completion remains server-evidenced. Show bounded
   pagination/coverage and explicit empty versus unavailable states.
4. Reuse native clarification/permission presenters with injected resolution
   transport, not a broad task client. On stale/expired actions refetch the source
   and show the actual result; link to the originating task/session. Keep pause
   and stop-managed-work separately labeled with truthful partial-stop feedback.
5. Add owner memory edit/forget controls with provenance/scope/revision conflicts,
   expiry and limitations on already-delivered context. Show credential health
   descriptors/unblock links but never secret values or raw resolver settings.
6. Make the essential chat/attention/actions usable on narrow mobile screens with
   keyboard/focus/scroll preservation and accessible labels. Keep details secondary;
   technical implementation status does not belong in normal product copy.
7. Localize all copy in the five existing locales and exercise pseudo-locale.
   Add hook/API/component tests for stale responses, conflicts and retry identity,
   and browser flows shared with task 11. Public docs must separate workspace
   Coordinator and personal Assistant routes and their available capabilities.

## Detailed evidence map

| Criterion | Planned evidence | Required edge cases |
| --- | --- | --- |
| AC-ORCHESTRATION-ASSISTANT-007.1 | Route/navigation and owner-switch tests | Flags, unauthenticated/foreign owner, unconfigured binding, private history |
| AC-ORCHESTRATION-ASSISTANT-007.2 | Panel/component tests and personal-assistant browser flow | Goal evidence, capabilities health, memory CAS/forget, native input resolution |
| AC-ORCHESTRATION-ASSISTANT-007.3 | `use-assistant`/API retry tests and dropped-ack browser case | Stable message ID, no duplicate action, reconnect gap, stale workspace response |
| AC-ORCHESTRATION-ASSISTANT-007.4 | `mobile-personal-assistant.spec.ts` | Narrow viewport, native input control, keyboard/focus, task return link |

Run typecheck, lint and `pnpm --dir apps/web run i18n:check` after targeted tests.
Fresh synthetic screenshots/video belong to task 11; no copied live conversations.

## Scope boundaries and delivery

This page does not replace the workspace Coordinator page or Kanban. Workspace
link administration is task 10; show only supported home-workspace behavior until
then. A finished-looking UI must not expose pending read-only or resolution APIs
without backend enforcement and feature gates.

## Parallelism

`sequential`

## Working mobile contract

Desktop uses the shared conversation renderer with a bounded attention/details
column. Phone uses focused Chat, Attention and Details views, following the
existing Coordinator route and `mobile-column-tabs` pattern. Global entry is the
shared app navigation sheet; dense content stays in the route rather than a
picker drawer. Each active pane owns its vertical scroll, uses the page shell's
dynamic viewport/safe area and exposes 44px phone actions. Shared domain hooks,
request identities and presenters preserve native behavior in both layouts.
Mobile E2E will enter through navigation, send a generic request, resolve native
input and inspect memory without horizontal overflow.

## Results

Implemented the app-level Assistant route, desktop sidebar, shared mobile sheet
and mobile board menu. The selected private conversation uses native chat with
stable retry IDs, paginated history and reconnect/foreground refresh. Scoped
observation generations discard old-owner responses. Transient read failures keep
drafts while disabling writes. Native live activity remains in the canonical store.

Attention injects a narrow native clarification transport, offers only current
provider permission options, preserves task/session/source identity and links to
the original task. Native task history displays assistant attribution. Details
contains objective evidence, activity, capability/credential metadata, and scoped
memory editing/forgetting with original-instruction provenance. All five locales
and generated pseudo/Traditional Chinese catalogs are synchronized.

Verification (2026-09-18; logs in the private local implementation evidence directory):

- The retry regression first failed because a confirmed conflict retained intent
  revision 4 after current intent became 6. After clearing only definite failed
  attempts, all four input-hook tests passed; ambiguous retries retain their ID.
  A separate skip-reason assertion was corrected to use the existing native
  `reject_reason` contract; that transport already preserved the reason.
- `pnpm --dir apps/web test app/assistant hooks/domains/orchestration
  hooks/domains/session/use-clarification-transport.test.ts
  hooks/domains/session/use-clarification-group.test.ts
  components/task/chat/messages/clarification-request-message.test.tsx
  components/task/chat/messages/permission-action-row.test.tsx
  components/task/chat/clarification-input-overlay.test.tsx
  lib/api/domains/assistant-api.test.ts
  lib/api/domains/orchestration-conversation-api.test.ts
  src/spa-routes.canvas.test.ts`: 91 tests in 16 files passed.
- Navigation regression tests cover app sidebar, shared sheet and mobile board
  menu. The enabled/disabled Assistant destination has a dedicated assertion.
- Managed host E2E, one worker, retries disabled: desktop personal-assistant flow
  passed (6.9s); Pixel 5 mobile flow passed (8.1s). Both use the real backend and a
  running native mock worker question, test-owned SQL objective/source fixtures,
  no HTTP stubs, and Office off. Selection creates no duplicate conversation
  session. Native answer, objective, memory provenance/forget and pause/resume
  converge. Disabled desktop route makes no assistant API requests.
- Phone E2E also checks pseudo-locale, 44px tabs and no horizontal overflow;
  `assistant-phone-pseudo.png` was visually inspected and stays in local E2E
  evidence. Initial E2E failures exposed a wrong test button label and missing
  mobile-board entry; both were corrected before the passing runs.
- Full Orchestration runtime race suite passed (15.262s), including the new
  provenance route's owner/runtime/feature-off checks. Go lint reports zero issues.
- Typecheck, changed-file ESLint, i18n validation, architecture checks, public-doc
  validator (48 pages) and its 61 tests pass. Backend schema is unchanged.

This work does not qualify a provider: the synthetic mock profile is correctly
shown as unsupported for restricted assistant execution. Task 11 supplies combined
scenarios, constrained-provider evidence and fresh shareable synthetic media.
Live Kandev and its data were not changed.
