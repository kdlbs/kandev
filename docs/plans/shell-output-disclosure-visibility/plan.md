---
spec: docs/specs/ui/requirements/acp-shell-command-output.md
created: 2026-08-07
status: done
---

# Implementation Plan: Hide empty shell output disclosures

## Root cause

`ToolExecuteMessage` always mounts `ShellOutputDisclosure`, including when the
projected shell summary has no stdout or stderr. The disclosure formats a
zero-byte summary as `Output`, so users can expand a control that only reveals
the empty-output or exit-status state. The summary already carries the data
needed to decide whether an output body exists.

## Overview

Guard the shared shell output disclosure at the command-row boundary using the
browser-facing summary. Preserve the existing collapsed, on-demand behavior for
commands with output, then prove both paths in the existing component, desktop,
and mobile chat tests. No backend, API, persistence, or mobile composition
change is required.

## Frontend

### Command row disclosure guard

Files:

- `apps/web/components/task/chat/messages/tool-execute-message.tsx`
- `apps/web/components/task/chat/messages/tool-execute-message.test.tsx`

Render `ShellOutputDisclosure` only when the normalized summary represents
retained stdout/stderr: `has_output` is true or either projected byte count is
positive. Keep the disclosure unmounted for zero-output commands so the
on-demand hook cannot fetch or poll accidentally. Commands with output keep
their existing collapsed default, endpoint fetch, polling, truncation, and exit
status behavior.

### Mobile parity contract

Desktop and mobile use the same `ToolExecuteMessage` data and disclosure guard;
mobile renders it inside the existing `TaskChatPanel` composition. The nearest
shipped mobile exemplar is
`apps/web/e2e/tests/chat/mobile-tool-execute-output.spec.ts`, which already
proves the long-command/output path. The mobile hierarchy remains the command
row first and the optional output disclosure second; a zero-output command has
no secondary action because there is no content to inspect. The chat transcript
continues to own vertical scrolling, with no new drawer, overlay, viewport, or
safe-area behavior.

## Tests

- **What:** A zero-output summary renders the command/status context without a
  `Show command output` control, while a summary with `has_output` or positive
  byte counts retains the collapsed disclosure and lazy-fetch behavior.
  **File:** `apps/web/components/task/chat/messages/tool-execute-message.test.tsx`.
  **How:** Extend the existing React Testing Library fixtures with an explicit
  zero-output case and keep the current output-state assertions.
- **What:** Desktop chat does not expose an empty output affordance, while a
  real-output command still fetches only after expansion.
  **File:** `apps/web/e2e/tests/chat/tool-execute-output.spec.ts`.
  **How:** Seed a shell message with an empty projected output summary, assert
  the command/status remain visible and the disclosure/request are absent, then
  retain the existing lazy completed-output scenario.
- **What:** Mobile chat has the same no-affordance behavior and preserves the
  existing long-command/output path inside the viewport.
  **File:** `apps/web/e2e/tests/chat/mobile-tool-execute-output.spec.ts`.
  **How:** Add a no-output seeded command assertion alongside the existing
  mobile output scenario.

## E2E Tests

- **Scenario:** GIVEN a projected shell command with no retained output, WHEN
  desktop chat renders it, THEN the command/status are visible and no output
  disclosure or shell-output request exists.
  **File:** `apps/web/e2e/tests/chat/tool-execute-output.spec.ts`.
- **Scenario:** GIVEN the same no-output command on the mobile chat surface,
  WHEN the transcript renders, THEN the empty disclosure is absent without
  changing the existing mobile command wrapping or output behavior.
  **File:** `apps/web/e2e/tests/chat/mobile-tool-execute-output.spec.ts`.

## Verification Results

- `cd apps && pnpm install --frozen-lockfile` — passed.
- `cd apps && pnpm --filter @kandev/web test -- --run components/task/chat/messages/tool-execute-message.test.tsx` — passed, 9 tests.
- `cd apps/web && pnpm run typecheck` — passed.
- `cd apps/web && pnpm e2e:run --project chromium tests/chat/tool-execute-output.spec.ts` — passed, 4 tests.
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-tool-execute-output.spec.ts` — passed, 2 tests.
- `pnpm exec prettier --check` on the changed TypeScript files — passed.
- `git diff --check` — passed.
- Managed desktop/mobile screenshot captures with synthetic data — passed; compressed assets are listed in `.pr-assets/manifest.json` and are not committed to the PR branch.

## Implementation Waves

Wave 1 (sequential):

- [x] [task-01-gate-empty-output-disclosure](task-01-gate-empty-output-disclosure.md) (done)

## Risks

- Older or partially populated summaries may omit `has_output`; positive byte
  counts must still keep the disclosure visible so valid output is not hidden.
- No new mobile interaction pattern is introduced; the only mobile change is
  removing an unusable control when there is no output body.

## Open Questions

None.
