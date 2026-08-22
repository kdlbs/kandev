---
id: "01-duration-helper"
title: "Duration helper with unit tests"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/prompt-turn-duration.md"
---

# Task 01: Duration helper with unit tests

- **Acceptance:**
  1. `messageTurnDurationSeconds(message: Message, turn: Turn | null): number | null` is exported from `apps/web/lib/prompt-history.ts`.
  2. It returns `null` unless the message is a user prompt (`author_type === "user"`) WITH a non-empty `turn_id` AND a turn that is the message's own (`turn.id === message.turn_id` AND `turn.session_id === message.session_id`, mirroring the panel's composite `session_id:id` resolution in `turnCompletionByPrompt`) whose `completed_at` parses; `Math.floor`-floors the elapsed `(completed_at − created_at)` to whole seconds and clamps at `0`. Unparseable timestamps yield `null`, never `NaN`. Table-test the `turn_id` guard and the identity mismatch explicitly (user message with NO `turn_id` → `null`; user message with `turn_id: ""` (empty string) plus a completed passed turn → `null` — presence/type checks alone must not accept an empty id; `turn.id` ≠ `message.turn_id` → `null`; `turn.session_id` ≠ `message.session_id` → `null`).
  3. Table-driven unit tests in `apps/web/lib/prompt-history.test.ts` cover every spec scenario for the helper, including an explicit regression case for the intentional divergence from the panel: two user messages sharing one completed turn return two different durations (each `completed_at − own created_at`), a user message with no `turn_id` returns `null` even when a completed turn is passed in, and a mismatched `turn.id`/`turn.session_id` returns `null` even when the passed turn is completed.
  4. The helper doc comment states that only the panel's timestamp parsing/floor/clamp arithmetic is shared — the panel's "earlier of turn completion and next prompt" bound does not apply (see plan's bound clarification).
- **Verification:**
  ```sh
  cd apps/web && pnpm vitest run lib/prompt-history.test.ts
  ```
- **Files likely touched:**
  - `apps/web/lib/prompt-history.ts` (add export; reuse private `epochNanoseconds`)
  - `apps/web/lib/prompt-history.test.ts` (new `describe` block)
- **Dependencies:** None.
- **Parallelism:** sequential.
- **Inputs:**
  - Spec: `docs/specs/ui/prompt-turn-duration.md` — `What` duration rule, all `Scenarios`.
  - Plan: `docs/plans/prompt-turn-duration/plan.md` — Frontend "Duration helper", Tests table.
  - Pattern: `buildPromptHistoryEntries` in `apps/web/lib/prompt-history.ts` shows the shared arithmetic for the completion bound (completion ns minus prompt ns, `BigInt(1_000_000_000)` division, clamp `BigInt(0)`) — reuse only that arithmetic and `epochNanoseconds`; do NOT apply the panel's next-prompt bound; `formatPromptDuration` (already exported, same file) stays as the formatter.
  - `Turn` type: `apps/web/lib/types/http.ts` — `completed_at?: string` is optional.
- **Output contract:** summary; files changed; tests run with counts; blockers/risks; update task status and `plan.md` checkbox.

## Results

- TDD red: `cd apps/web && pnpm vitest run lib/prompt-history.test.ts` failed as expected: 5 assertions received `null` before `messageTurnDurationSeconds` existed.
- Verification: `cd apps/web && pnpm vitest run lib/prompt-history.test.ts` passed: 1 file, 39 tests.
- Generated artifacts: none. Cleanup: no generated artifacts to remove.
