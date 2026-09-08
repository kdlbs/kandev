---
spec: "../../specs/ui/requirements/subagent-observability.md"
status: building
created: 2026-08-03
---

# Plan: Subagent Observability

Five slices. Task 1 is backend-only; Tasks 2, 4, and 5 are independent. Task 3
depends on Tasks 1 and 2 because it consumes the captured result data. Ordered
by value.

Task detail is inline rather than in sibling `task-NN-*.md` files. Each slice
may change implementation, tests, locale files, or types, with one verification
command documented per slice.

| # | Task | Layer | Depends on |
|---|---|---|---|
| 1 | Capture subagent result text and model | backend | — |
| 2 | Keep subagents out of turn groups | web | — |
| 3 | Surface the result on the card | web | 1 (for data), 2 (for reachability) |
| 4 | De-duplicate card metadata | web | — |
| 5 | Board subagent chip | web | — |

## Task 1 — Capture subagent result text and model

**Acceptance:** `claudeSubagentResponse` reads `content` and `resolvedModel` off
`_meta.claudeCode.toolResponse`. Text blocks concatenate newline-separated in
order into `ResultText`; non-text blocks are skipped; absent/empty/all-non-text
content yields an empty `ResultText` with no placeholder. `resolvedModel`
populates `Model`. The existing merge rule (a set value is not overwritten by a
later empty one) covers both fields.

**Files:** `apps/backend/internal/agentctl/server/adapter/transport/acp/subagent.go`,
`subagent_test.go`.

**Verification:** `make -C apps/backend test` (package `./internal/agentctl/...`).

## Task 2 — Keep subagents out of turn groups

**Acceptance:** turn grouping emits subagent messages at the group's own level in
chronological position; only non-subagent tool calls are collapsed. A group with
no remaining tool calls emits no collapsed row. The collapsed label's leading
numeral is the collapsed tool-call count and pluralizes with it; it never
mentions subagents.

**Files:** `apps/web/hooks/use-processed-messages.ts`,
`apps/web/components/task/chat/messages/turn-group-message.tsx`, plus their tests.

**Verification:** `cd apps/web && pnpm vitest run hooks/use-processed-messages.test.ts components/task/chat/messages/turn-group-message.test.tsx`.

## Task 3 — Surface the result on the card

**Acceptance:** the collapsed card shows a single-line summary — the first
non-empty line of `result_text`, ellipsized — and the expanded body shows the
full text above any child messages. No `result_text` means no summary line and no
placeholder. The description suppresses a leading restatement of the subagent
type, not only an exact match.

**Files:** `apps/web/components/task/chat/messages/tool-subagent-message.tsx`
and tests; new copy keyed into `apps/web/src/locales/en/chat.json`.

**Verification:** `cd apps/web && pnpm vitest run components/task/chat/messages/tool-subagent-message.test.tsx`.

## Task 4 — De-duplicate card metadata

**Acceptance:** the `tools` chip is omitted when the reported `tool_use_count`
equals the rendered child count, and kept when they diverge.

**Files:** `apps/web/components/task/chat/messages/subagent-meta.ts`,
`subagent-meta-row.tsx`, `subagent-meta.test.ts`.

**Verification:** `cd apps/web && pnpm vitest run components/task/chat/messages/subagent-meta.test.ts`.

## Task 5 — Board subagent chip

**Acceptance:** a kanban card renders a count chip while `activeSubagentCount > 0`
and nothing at zero or absent. Driven by the existing store field; no new API.

**Files:** `apps/web/components/kanban-card-content.tsx`,
`apps/web/components/kanban-card-status-icon.test.tsx`; copy in
`apps/web/src/locales/en/common.json`.

**Verification:** `cd apps/web && pnpm vitest run components/kanban-card-status-icon.test.tsx`.

## Whole-feature validation

- `make -C apps/backend test lint`
- `cd apps/web && pnpm run typecheck && pnpm lint && pnpm vitest run`
- `pnpm run i18n:check` and `pnpm run i18n:ratchet`
- Local deploy: run the dev backend + SPA and confirm the B1 task
  (`93ff1109-2a32-41c3-a550-dadf529395f6`) shows its four reviewer cards without
  expanding a turn group.
