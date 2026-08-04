---
id: "07-composer-steer-affordance"
title: "Show a delivery affordance in the composer"
status: done
wave: 4
depends_on: ["06-session-steer-contract"]
plan: "plan.md"
spec: "../../specs/platform/mid-turn-steering.md"
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
