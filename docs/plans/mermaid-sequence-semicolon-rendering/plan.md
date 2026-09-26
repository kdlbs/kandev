---
spec: docs/specs/ui/requirements/mermaid-rendering.md
created: 2026-07-30
status: complete
---

# Implementation Plan: Mermaid Sequence-Message Semicolon Rendering

## Overview

Extend the shared Mermaid source normalizer so literal semicolons in sequence-message prose are
encoded as `#59;` before rendering. Keep Mermaid's valid use of semicolons as inline statement
separators intact, and pin the reported diagram plus preservation cases with focused unit tests. Add
a shared failure reporter used by both chat and task-plan renderers so a rejected render leaves one
copyable console entry containing the parser error, complete original source, and normalized source
when it differs. Finally, route both toast paths through one task-scoped in-memory registry so
refocusing a task cannot replay the same class of error toast.

## Confirmed root cause

Mermaid sequence diagrams allow semicolons to replace line breaks. In the reported line
`State->>State: Locate numeric code string; preserve message unchanged`, Mermaid treats the
semicolon as a statement boundary and then parses `preserve message unchanged` as malformed
sequence syntax. Mermaid 11.16.0 reproduces the line-13 `got 'NEWLINE'` error. Replacing the literal
semicolon with `#59;` or a comma parses successfully with both LF and CRLF input.

## Frontend

### Shared Mermaid normalization

- Update `sanitizeMermaidCode` in `apps/web/components/shared/mermaid-utils.ts` through a focused
  sequence-diagram helper.
- Detect sequence-diagram content without changing other diagram types.
- On each physical line, distinguish a message-text semicolon from a semicolon whose suffix starts
  another valid sequence statement. Encode only message-text semicolons as `#59;`.
- Preserve line endings, existing entity escapes, and the existing quote-aware bracket, edge-label,
  and stadium-label passes.

### Failed-render diagnostics

- Add a shared failure-reporting helper in `apps/web/components/shared/mermaid-utils.ts`.
- Emit one persistent `console.error` per non-cancelled rejected render with a searchable `[mermaid]`
  prefix. Use one flat multiline string containing the parser error and full original source; append
  the full normalized source only when it differs from the original.
- Call the helper from both current rejection paths:
  `apps/web/components/shared/mermaid-block.tsx` for chat Markdown and
  `apps/web/components/editors/tiptap/tiptap-mermaid-extension.ts` for task plans.
- Keep toast content and inline-error behavior unchanged. The existing console interceptor mirrors
  the entry into Kandev's in-memory frontend log buffer, making it available in browser logs and
  user-initiated diagnostic exports without automatically sending it to the backend.

### Task-scoped toast deduplication

- Add a shared once-per-task toast gate in
  `apps/web/components/shared/mermaid-error-toast.tsx`. Keep the set at module scope so it survives
  Mermaid component and task-panel remounts during one frontend runtime.
- Route both the chat `MermaidBlock` rejection path and task-plan `MERMAID_ERROR_EVENT` listener
  through the same gate.
- Read and capture the active task ID in both failure paths. When no task is focused, retain the
  existing unsuppressed behavior.
- Suppress only the toast: every non-cancelled rejection still logs its full source and retains
  existing inline error/recovery behavior.

### Mobile parity

This changes only source normalization inside the existing shared renderer. It does not change
rendered layout, controls, navigation, scrolling, pointer behavior, or responsive composition.
Focused utility tests cover the shared desktop/mobile behavior; no new mobile Playwright scenario
is required.

## Tests

- Add a RED regression case to `apps/web/components/shared/mermaid-utils.test.ts` using the full
  reported sequence diagram and assert that only the message-text semicolon becomes `#59;`.
- Cover an already escaped `#59;`, LF and CRLF inputs, a valid inline separator between sequence
  statements, and a non-sequence diagram whose semicolon must remain unchanged.
- Add a focused console-reporting test that asserts the complete original source, parser error,
  conditional normalized source, and one-entry-per-failure behavior.
- Extend `apps/web/components/shared/mermaid-block-streaming.test.tsx` to prove a rejected chat
  render invokes the reporter without changing toast recovery behavior.
- Extend `apps/web/components/shared/mermaid-error-toast.test.tsx` to prove duplicate events for one
  task—including an unmount/remount that models focus changes—show one toast, while another task
  can show its first toast and non-task errors retain existing behavior.
- Prove chat and task-plan paths share the same registry so two failing diagrams across those
  surfaces do not produce two toasts for one task.
- Run the focused Vitest files and changed-file ESLint. Each regression must fail before its
  implementation for the expected reason and pass after the minimal change.

## E2E tests

No new E2E test is planned. The failing behavior is a pure input-normalization defect in a shared
utility, existing chat and plan renderers already consume that utility, and no UI interaction or
viewport behavior changes.

## Implementation

- [x] [task-01-normalize-sequence-message-semicolons](task-01-normalize-sequence-message-semicolons.md)
- [x] [task-02-log-failed-diagram-source](task-02-log-failed-diagram-source.md)
- [x] [task-03-deduplicate-task-diagram-toasts](task-03-deduplicate-task-diagram-toasts.md)

## Verification

```bash
cd apps
pnpm install --frozen-lockfile
pnpm --filter @kandev/web test -- \
  components/shared/mermaid-utils.test.ts \
  components/shared/mermaid-block-streaming.test.tsx \
  components/shared/mermaid-error-toast.test.tsx
pnpm --filter @kandev/web exec eslint \
  components/shared/mermaid-utils.ts \
  components/shared/mermaid-utils.test.ts \
  components/shared/mermaid-block.tsx \
  components/shared/mermaid-block-streaming.test.tsx \
  components/shared/mermaid-error-toast.tsx \
  components/shared/mermaid-error-toast.test.tsx \
  components/editors/tiptap/tiptap-mermaid-extension.ts \
  --max-warnings 0
```

## Risks

- A literal semicolon is ambiguous because Mermaid accepts it both inside intended prose and as a
  statement separator. The helper must preserve separators whose suffix is recognizable sequence
  syntax instead of blindly escaping every semicolon after a message colon.
- Sequence syntax has several statement forms. Tests must protect at least message-to-message
  separators while the implementation keeps classification local and conservative.
- The normalizer must not convert the semicolon that terminates an existing entity escape such as
  `#59;`.
- Full diagram source can contain sensitive application text. The user explicitly requested the
  complete source for diagnosis; keep it in the browser console/in-memory frontend buffer and do
  not add a new transport or automatic upload.
- Both independent renderers must report through the same helper so log formatting and
  copyability do not drift.
- A component-local ref would reset when task panels remount and reproduce the bug. Toast history
  must outlive those mounts but remain frontend-runtime-local rather than becoming persisted task
  state.
- Task identity must be captured with the rejection/event handling so task-panel remounts share
  history and a later task focus does not reuse a component-local key.

## Out of scope

- Replacing Mermaid's parser or adding a generic parse-and-retry pipeline.
- Repairing unrelated invalid diagram constructs.
- Altering toast copy or visual presentation, diagram controls, or responsive layout.
- Redacting the requested full diagram source from failed-render logs.
