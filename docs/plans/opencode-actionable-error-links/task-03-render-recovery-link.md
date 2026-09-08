---
id: "03-render-recovery-link"
title: "Render recovery links"
status: done
wave: 3
depends_on: ["02-persist-recovery-link"]
plan: "plan.md"
spec: "../../specs/agents/requirements/agent-stall-recovery.md"
---

# Task 03: Render Recovery Links

## Acceptance

- Kanban action messages and persistent last-agent-error notices render a
  localized external link when `remediation_url` is present.
- Office failed-session entries render the same link from initial API data and
  live `session.state_changed` metadata.
- Browser-side defense-in-depth validation rejects invalid URLs before creating
  an `href`; valid links use `target="_blank"` and
  `rel="noopener noreferrer"`.
- The existing sanitized message, collapsed technical details, recovery
  buttons, keyboard navigation, and mobile chat scroll owner remain intact.

## Verification

```text
cd apps && pnpm --filter @kandev/web test -- --run components/task/chat/messages/action-message.test.tsx lib/session-last-agent-error.test.ts components/task/simple/chat-entries.test.ts lib/remediation-url.test.ts
cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet
```

Add focused tests for generic/quota cards, dismissed notices, Office mapping,
invalid metadata, translated labels, and the minimum 44px phone affordance.

## Files likely touched

- `apps/web/lib/session-last-agent-error.ts`
- `apps/web/components/task/chat/messages/action-message.tsx`
- `apps/web/components/task/chat/messages/action-message.test.tsx`
- `apps/web/components/task/chat/message-list-shared.tsx`
- `apps/web/app/office/tasks/[id]/page.tsx`
- `apps/web/app/office/tasks/[id]/types.ts`
- `apps/web/components/task/simple/chat-entries.ts`
- `apps/web/components/task/simple/components/run-error-entry.tsx`
- focused Office/chat tests
- `apps/web/src/locales/en/task.json`
- `apps/web/src/locales/en/chat.json`
- matching pseudo-locale files

## Dependencies and risks

Task 02. The external URL is a user-visible security boundary; keep validation
at the backend and browser edges. Do not render the raw diagnostic message as
HTML or use translated text for control flow.

## Results

Implemented 2026-08-07.

- `lib/remediation-url.ts` mirrors the backend allowlist at the browser edge
  (host, scheme, path shape, bounded identifier, no port/query/fragment/
  userinfo); `components/task/remediation-link.tsx` renders the localized
  `chat:openProviderPage` link with `target="_blank"`, `rel="noopener noreferrer"`
  and a 44px phone touch target, and returns null for invalid input.
- Wired into `ActionMessageDetails` (generic + quota recovery cards), the
  persistent `LastAgentErrorNotice` (`readLastAgentError` reads
  `remediation_url`/`remediationUrl`), and Office `RunErrorEntry` via
  `buildRunErrorsFromSessions`; the office page maps `metadata` from the API
  and merges live `session.state_changed` metadata.
- Unit tests: validator table, action-message link cases (quota/generic/
  invalid), notice link cases, session-last-agent-error parsing, and
  `buildRunErrorsFromSessions` preservation.

Verification: targeted vitest run (106 tests) and the full web unit suite
(1236 files / 9669 tests) passed; `typecheck`, `i18n:check`, `i18n:ratchet`
clean (en/pt-pt/zh-cn catalogs + regenerated pseudo).
