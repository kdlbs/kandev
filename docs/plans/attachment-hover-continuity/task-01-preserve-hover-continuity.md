---
id: attachment-hover-continuity-01
status: done
title: Preserve composer attachment hover continuity
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-TASKS-PROMPT-ATTACHMENTS-001
acceptance_criteria:
  - AC-TASKS-PROMPT-ATTACHMENTS-001.15
system_design:
  - ../../specs/tasks/system-design/prompt-attachments.md
---

# Preserve composer attachment hover continuity

## Summary and scope

Repair the shared ContextChip pointer interaction so users can reach image and
file attachment preview controls. The zero close delay closes the preview while
the pointer crosses its 4 px offset. Set an explicit 300 ms close delay and add
coverage in `apps/web/components/task/chat/context-items/context-chip.test.tsx`.
The production change is in the adjacent `context-chip.tsx`.

Exclude upload, persistence, delivery, layout, and touch-drawer changes. This
work has no dependencies. The existing Radix timer owns cancellation and cleanup.

## Acceptance

1. A 250 ms pointer crossing retains the same open preview and its actionable control.
2. Entering the preview cancels pending closure; leaving both surfaces closes it.
3. Existing coarse-pointer and image-inspection behavior remains covered.

## ASCII UI preview

UI-01, matching [the plan](plan.md#ascii-ui-preview):

```text
[Preview image]
[Prompt] [File]
     ^ pointer crosses gap without closing preview
[Image attachment] [Remove]
[Message composer]
```

Phone composition and touch activation remain unchanged.

## Verification

From `apps/web/`:

```sh
pnpm exec vitest run components/task/chat/context-items/context-chip.test.tsx components/task/chat/context-items/image-item.test.tsx
```

From the repository root:

```sh
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Results

The original immediate-close code fails the new pointer-gap assertion. The
correction passes all 7 tests in the two targeted files. Live Chromium verified
50 ms and 250 ms vertical crossings and a 150 ms diagonal crossing without close
or removal events, working Prompt/File controls, and normal closure on exit.
The explicit delay pins the tested timing instead of relying on a library default.
