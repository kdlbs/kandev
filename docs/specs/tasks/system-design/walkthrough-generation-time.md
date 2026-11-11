---
status: current
system: tasks
requirements:
  - REQ-TASKS-WALKTHROUGH-TIME-001
---

# Walkthrough Generation Time System Design

## Ownership and existing contracts

Tasks owns the persisted walkthrough revision. This design exposes its existing timestamp in the shared walkthrough card.
The adjacent [navigation contract](../../ui/requirements/walkthrough-navigation-layout.md) continues to own responsive navigation behavior.
External [PR walkthroughs](../../ui/requirements/pr-walkthrough.md) have a separate lifecycle and are excluded.

`CreateTaskWalkthrough` in `apps/backend/internal/task/repository/sqlite/walkthrough.go`
sets `updated_at` on every upsert. A replacement preserves the original `created_at`.
Therefore, `TaskWalkthrough.updated_at` identifies the latest saved generation.
Using `created_at` would incorrectly date a replacement as the first walkthrough.

## Data flow

The existing HTTP backfill and WebSocket handlers populate `walkthroughs.byTaskId`.
`WalkthroughStepInner` reads the active task's walkthrough from that store.
Pass `updated_at` into `StepHeader`. Render the timestamp below the badge and progress text, before the step picker.
Store replacements update the timestamp through the existing subscription.
No extra request, timer, client timestamp, or persistence migration is needed.

## Presentation and localization

Add `WalkthroughGenerationTime` beside `walkthrough-step-card.tsx`.
Render a semantic `<time dateTime={updatedAt}>` with the test ID `walkthrough-generated-at`.
Validate with the existing `parseStrictRfc3339Timestamp` helper before formatting.
Return no timestamp for an absent, blank, or invalid value, including impossible calendar dates.
Do not fall back to `created_at` or the current clock.

Use `formatDateTime` from `apps/web/lib/i18n/formats.ts` with explicit year,
short month, day, hour, and minute options.
The native `title` shows the same localized timestamp with `timeZoneName: "short"`.
Timezone disclosure is supplementary, so phones require no extra interaction.
Leave the timezone option unset to use the browser's timezone.
Subscribe through `useTranslation()` so language changes rerender the value.
Use a `diff:walkthroughGeneratedAt` translation such as `Generated {{date}}`.
Supply the key in all seven supported languages through the existing locale workflow.

Use explicit `text-left` to override inherited alignment and align the timestamp with the badge text by including its padding and border inset.
Reuse `text-xs text-muted-foreground` and the header's existing horizontal `px-3` spacing.
Group progress and timestamp in one column with a `gap-1` inset. The close button spans both lines.
This uses the height already reserved for the 44px phone close target, preserving short-screen navigation.
Keep the drag handle on the progress row only, so timestamp text can be selected.
Allow wrapping, use no fixed width, and do not add an icon or animation.
The timestamp remains above the existing scrollable content.

The desktop floating card and phone bottom-sheet panel already share `WalkthroughStepInner`.
The nearest shipped mobile exemplar is `WalkthroughFloatingWindow` itself.
Retain its bottom placement, safe-area clearance, scroll ownership, and close/navigation controls.
The timestamp also appears in the inline card through the same component.
This is a content addition inside the existing surfaces, with no new interaction or breakpoint.

## Failure and compatibility

Malformed data must not cause `Intl.DateTimeFormat` to throw during rendering.
Missing timestamps leave the existing card usable without an empty metadata row.
An ordinary locale change reformats the same instant.
No permission, event, backend write, or logging behavior changes.

## Requirement mapping

| Criteria | Design responsibility |
| --- | --- |
| AC-TASKS-WALKTHROUGH-TIME-001.1, .2 | Existing `updated_at` and reactive store subscription |
| AC-TASKS-WALKTHROUGH-TIME-001.3 | Shared formatter and translated label |
| AC-TASKS-WALKTHROUGH-TIME-001.4 | Shared timestamp component, wrapping, and existing mobile surface |
| AC-TASKS-WALKTHROUGH-TIME-001.5 | Validation before formatting |

## Implementation plan

- [Plan and verification](../../../plans/walkthrough-generation-time/plan.md)
