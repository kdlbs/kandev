---
status: complete
requirements:
  - REQ-AGENTS-RICH-DIFF-PREVIEW-001
system_design:
  - ../../specs/agents/system-design/rich-diff-preview.md
created: 2026-11-25
---

# Rich diff preview

Deliver [one vertical work order](task-01-render-diff.md) covering the [requirements](../../specs/agents/requirements/rich-diff-preview.md) and [system design](../../specs/agents/system-design/rich-diff-preview.md). Use the existing workspace file flow. The main risks are confusing patch headers with hunk content and losing access to changes beyond the first page.

## ASCII UI preview

UI-01. Expanded preview on desktop and phone. Side by side is selected on first open.

```text
[Side by side *] [Unified]
Before                  | After
@@ -1 +1 @@
- "eventId": 141402386  | + "eventId": 141403809
```

UI-02. The reader selects Unified in the same card.

```text
[Side by side] [Unified *]
@@ -1 +1 @@
- "eventId": 141402386
+ "eventId": 141403809
```

On phones, place the file actions below the path and the layout selector above the patch. Keep the same column order, allow horizontal scrolling inside the patch, and place paging below it. The asterisk illustrates the selected state.

## Verification

The work order pairs a classifier regression test with desktop and phone transcript flows. Both browser flows and all 80 focused unit tests passed.
