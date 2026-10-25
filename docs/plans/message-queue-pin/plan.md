---
spec: docs/specs/ui/requirements/message-queue-pin.md
created: 2026-08-12
status: building
---

# Implementation Plan: Pin the Message Queue Panel

## Overview

Add a per-session pin toggle to the expanded message queue panel header
(placed between **Clear all** and **X**), persisted in `localStorage`. While
pinned, the queue panel opens automatically on mount so navigating away and
back (or reloading) no longer forces the user to click the queue chip. The
change is frontend-only: a small localStorage-backed hook, a header button,
open-state integration in `QueueAffordance`, i18n keys, unit tests, and one
E2E spec. No backend contract changes.

---

## Frontend

### New hook: `apps/web/hooks/use-queue-pinned.ts` (+ `.test.ts`)

Per-session boolean preference, modeled on the existing
`useLocalStorageBoolean` pattern (`useSyncExternalStore` + `storage` event +
custom sync event):

- Storage key `kandev:queue:pinned:<session_id>:v1`; absent/unreadable → `false`.
- Sync event name: `kandev:queue:pinned-changed`.
- Accept `sessionId: string | null`; when `null`, return `{ value: false,
  setValue: noop }` without touching storage.
- `setValue(next)` writes the literal `"true"`/`"false"` and broadcasts the
  sync event; failed writes throw like `useLocalStorageBoolean` (UI callers
  catch and ignore to satisfy the spec's silent-degradation failure mode).

### `apps/web/components/task/chat/queued-ghost-panel-header.tsx`

- New props: `pinned: boolean`, `onTogglePin: () => void`.
- Insert a toggle button between the **Clear all** button and the **X** close
  button:
  - `data-testid="queue-pin"`, `aria-pressed={pinned}`,
    `title`/`aria-label` from `t("chat:pinQueuedMessages")` /
    `t("chat:unpinQueuedMessages")`.
  - Icon: `IconPinned` when pinned, `IconPin` otherwise (verify names against
    the installed `@tabler/icons-react`; fall back to filled/outline pair that
    exists).
  - Same sizing/touch classes as the sibling buttons
    (`h-6 px-1.5` desktop; `[@media(pointer:coarse)]:h-11
    [@media(pointer:coarse)]:w-11`).

### `apps/web/components/task/chat/queued-ghost-list.tsx`

- `useQueuePanelOpenState(sessionId, entryCount, pinned)`:
  - Initial `isOpen` = `pinned && entryCount > 0`.
  - Session switch → `setIsOpen(pinned && entryCount > 0)` for the new session.
  - Entry-count drop to `0` → close (unchanged).
- `QueueAffordance`: call `useQueuePinned(sessionId)`, pass `pinned` and a
  toggle handler (`setValue(!pinned)`, errors swallowed) into
  `useQueuePanelOpenState` and through `QueuePanelDisclosure`/`QueuePanel` to
  `QueuePanelHeader`.

### i18n (`apps/web/src/locales/{en,pseudo,pt-pt,zh-cn}/chat.json`)

- `"pinQueuedMessages": "Pin message queue"` (+ pseudo/pt-pt/zh-cn).
- `"unpinQueuedMessages": "Unpin message queue"` (+ pseudo/pt-pt/zh-cn).

---

## Tests

### Unit — `apps/web/components/task/chat/queued-ghost-list.test.tsx`

Existing suite mocks `useQueue` and renders `QueueAffordance` inside
`StateProvider`; `useQueuePinned` needs a localStorage mock (`makeLocalStorageMock`
helper, as in `use-local-storage-boolean.test.ts`). Behaviors:

- **What:** pin toggle renders between Clear all and X in the expanded panel.
  **How:** open panel via chip click, assert `queue-pin` exists and its DOM
  position sits between `queue-clear-all` and `queue-close`.
- **What:** clicking pin sets `aria-pressed=true` and persists; clicking again
  reverts. **How:** fireEvent.click + assert `aria-pressed`, then assert the
  storage key holds `"true"` / `"false"`.
- **What:** pinned + entries remount → panel opens without chip. **How:**
  set localStorage before render, assert `queued-ghost-list` present and
  `queue-chip` absent.
- **What:** unpinned remount → chip shown, panel closed (regression guard).
- **What:** pinned + zero entries → no chip, no panel.
- **What:** session-switch remount follows the new session's pin
  (pin `sess-1`, render with `sess-2` unpinned → closed; pinned `sess-2` →
  open).

### Unit — `apps/web/hooks/use-queue-pinned.test.ts`

- Defaults `false` when absent; reads literal `"true"` only.
- `null` session → `{ value: false }` and no storage access.
- `setValue` persists and updates; storage failures degrade to default.

### E2E — `apps/web/e2e/tests/chat/message-queue-pin.spec.ts`

Pattern from `apps/web/e2e/tests/chat/message-queue.spec.ts` (queue a
message, open via `queue-chip`, navigate with `page.goto`).

- **Scenario:** queue a message → expand panel → click `queue-pin` → navigate
  away (`/`) and back to `/t/<id>` → panel `queued-ghost-list` visible and
  `queue-chip` absent without clicking anything.
- **Scenario:** without pinning, the same navigation leaves the panel
  collapsed (chip visible) — regression guard.

---

## E2E Tests

See the two scenarios above in
`apps/web/e2e/tests/chat/message-queue-pin.spec.ts`; run with
`cd apps/web && pnpm e2e:raw -- tests/chat/message-queue-pin.spec.ts` (after
the e2e fixture backend is available per `e2e/README.md`).

---

## Verification Results

- `cd apps && pnpm --filter @kandev/web test -- hooks/use-queue-pinned.test.ts components/task/chat/queued-ghost-list.test.tsx components/task/chat/queued-ghost-pin.test.tsx` → 56/56 passed.
- `cd apps/web && pnpm run typecheck` → passed.
- `cd apps/web && pnpm run lint` → passed (0 warnings).
- `cd apps/web && pnpm run i18n:check` + `i18n:ratchet` → passed.
- `make fmt` (repo) → no changes; `make typecheck` (all TS apps) → passed; `make lint` (backend Go + web + harness + architecture) → passed.
- E2E (managed runner, production build): `tests/chat/message-queue-pin.spec.ts` 2/2; `--project mobile-chrome tests/chat/mobile-message-queue-management.spec.ts` 3/3 (incl. new mobile pin test).
- `make test` full-suite: backend failures are pre-existing/environmental (zero Go changes; launcher tests assert a Homebrew install path absent on this machine — reproduced on clean HEAD). Web suite: all queue/pin suites pass; `http-git-server.test.ts` failures reproduce on clean HEAD; the remaining sentry/file-browser/i18n failures pass in isolation (load flakiness), none import the changed files.

### Adversarial review loop (10-luna-review-fix)

Rounds 1–3 produced findings (storage-sync pin flip, empty→nonempty reopen,
pinned-empty unpin leak), each fixed, tested, and committed (`dabb6c80e`,
`8fb2a3de3`, `fdaf84ee9`); round 4 returned `NO_FINDINGS` and the loop
stopped. Final unit count 63/63; desktop pin E2E 2/2 and mobile pin E2E 1/1
re-run green against the final code.

---

## Implementation Waves And Parallel Candidates

Sequential; frontend change must land before its E2E.

```
Wave 1:
- [x] [task-01-frontend-pin](task-01-frontend-pin.md)

Wave 2:
- [x] [task-02-e2e-pin](task-02-e2e-pin.md)
```

---

## Open Questions
None.
