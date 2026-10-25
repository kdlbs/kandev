---
id: "07-composer-steer-affordance"
title: "Show a delivery affordance in the composer"
status: done
wave: 4
depends_on: ["06-session-steer-contract"]
plan: "plan.md"
spec: "../../specs/platform/requirements/mid-turn-steering.md"
---

# Task 07: Show a delivery affordance in the composer

- **Acceptance:** While a session reports `supports_steering` and is generating
  with an empty queue, the composer indicates the message will be **delivered
  now rather than held**, and submitting sends it. When `supports_steering` is
  false, or a message is already queued, the composer keeps today's queue
  affordance and today's behavior byte-for-byte.
- **Acceptance:** The copy commits only to delivery, never to folding into the
  running turn — the agent CLI underneath may defer it, and that outcome renders
  as an ordinary queued message arriving, with no error and no version warning.
- **Acceptance:** All new copy goes through `t()` / `<Trans>` with new keys; no
  hardcoded literal, no `t()` at module scope, no English plural ending passed
  as a value, and no translated string compared with `===`.
- **Verification:** `cd apps/web && pnpm run typecheck && pnpm lint && pnpm test && pnpm run i18n:check && pnpm run i18n:ratchet`
- **Files likely touched:** the chat composer component and its hook under
  `apps/web/components/`, the session store selector under
  `apps/web/lib/state/slices/`, and the relevant locale resource files.
- **Dependencies:** Task 06.
- **Inputs:** Spec "What" (the delivery-not-folding promise) and the plan's "Why
  there is no compatibility branch" — the honesty constraint is the reason this
  task exists as its own step. Root `CLAUDE.md` i18n rules are enforced by
  pre-commit and CI, not advisory.
- **Risks:** A SCREAMING_CASE config table of labels passes the i18n lint rule
  silently — review any such table by eye. Check the result in the pseudo-locale
  (Settings → General → Appearance in a dev build); a clean lint is not proof the
  strings are localized.
- **Output contract:** Report the affordance states and their conditions, the new
  i18n keys, the pseudo-locale check, exact commands/results, and update only
  this task's status.

## Validation Results

Re-run on 2026-08-06 against the branch merged with `main`. Supersedes the
2026-08-04 entry below, which certified "or a message is already queued, the
composer keeps today's queue affordance" (AC1) without that half being
implemented or tested.

- An independent Codex review on 2026-08-06 confirmed the gap by tracing the
  full chain: `SteerEligible` on the backend computes `supports_steering`
  without a queue-length check, and every frontend hop
  (`deriveSessionFlags` → `use-chat-panel-state.ts` → `use-composer-props.ts`
  → `use-chat-input-container.ts`) passed `supportsSteering` straight through
  with no reference to the queue. A session with `supports_steering: true`
  and an already-queued message showed "Send now — delivered to the running
  turn" even though `SteerTask` would silently queue the next send behind the
  existing one (correct backend behavior, wrong displayed affordance).
- Fixed with `resolvesSteeringAffordance(supportsSteering, queuedCount)`
  (`hooks/domains/session/session-input-mode.ts`), applied in
  `use-chat-panel-state.ts` where session state and live queue count
  (`useQueue`) are already both in scope. AC1 is now fully satisfied.
- `cd apps/web && pnpm run typecheck`: passed.
- `cd apps/web && npx eslint hooks/domains/session/session-input-mode.ts hooks/domains/session/session-input-mode.test.ts components/task/chat/use-chat-panel-state.ts --max-warnings 0`: passed, 0 issues.
- `cd apps/web && npx vitest run hooks/domains/session/session-input-mode.test.ts hooks/domains/session/use-session-state.test.ts components/task/chat/use-chat-input-container.test.ts`: 48/48 passed, including three new `resolvesSteeringAffordance` cases.
- `cd apps/web && pnpm run i18n:check`: passed — no new copy was added (this
  is a logic-only fix), so no new keys; the pre-existing
  `chat:composerSteerPlaceholder` orphan warning (a checker false-positive
  from the scoped `useTranslation("chat")` call site, noted below) is
  unchanged.
- `cd apps/web && npx playwright test --config e2e/playwright.config.ts tests/chat/mid-turn-steering.spec.ts --project=chromium --workers=1`: all 6 tests passed, including "keeps a steer behind a message that was already queued," which now also exercises the corrected affordance's underlying data path end to end (the placeholder text itself is intentionally not asserted in E2E — see that spec file's own top comment on why it's a flaky signal).

---

Re-run on 2026-08-04 against the branch merged with `main`.

- `cd apps/web && pnpm run typecheck`: passed.
- `cd apps/web && pnpm lint`: passed (`eslint --max-warnings 0`).
- `cd apps/web && pnpm test`: passed for every steering-related suite —
  `use-chat-input-container.test.ts` (7/7, including the steer-placeholder
  precedence case) and `session-input-mode.test.ts`. The 6 failures in the full
  run are pre-existing on clean `main` in untouched files (see task-02's record).
- `cd apps/web && pnpm run i18n:check`: passed — keys OK, `<Trans>` indices OK,
  no inline English plurals, no module-scope `t()`.
- `cd apps/web && pnpm run i18n:ratchet`: passed — 0 added + 9 modified files
  clean, guard allowlist intact (243 entries).
- New key: `chat:composerSteerPlaceholder`, present in `en` and in sync in the
  pseudo-locale (confirmed by `i18n:check`).
