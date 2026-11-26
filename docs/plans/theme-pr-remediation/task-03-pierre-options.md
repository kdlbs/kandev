---
id: "03-pierre-options"
title: "Preserve Pierre rendering options"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-APP-THEMES-001
acceptance_criteria:
  - AC-UI-APP-THEMES-001.3
system_design:
  - ../../specs/ui/system-design/application-themes.md
---

# Task 03: Preserve Pierre Rendering Options

## Summary

Keep diff configuration intact during built-in and contributed theme changes.
Merge the selected theme into options read from the real pool at execution
time, inside the existing serialized/coalesced switch operation.

## In scope

- Extend the narrow pool type to include its public option reader.
- Preserve `lineDiffType`, tokenizer flags and both line-length limits when
  calling the replacement-style `setRenderOptions` API.
- Add a real-manager regression using zero workers and lazy text-only setup;
  update the controlled serialization fake with the public reader contract.
- Retain stale-switch coverage and verify that options changed while a theme
  switch waits in the queue are not replaced by an earlier captured snapshot.

## Out of scope

New renderers, worker pool recreation, private-library mutations and UI layout.

## Acceptance

1. The configured `word` algorithm and other non-default options survive
   built-in/custom changes and fallback to built-in colors.
2. An older pending switch cannot win; the latest operation preserves the
   current options when it actually executes.

## Verification

```bash
(cd apps/web && pnpm exec vitest run lib/theme/pierre-theme.test.ts lib/theme/theme-adapters.test.ts)
(cd apps/web && pnpm exec eslint lib/theme/pierre-theme.ts lib/theme/pierre-theme.test.ts)
```

Add `preserves non-theme options with the real Pierre manager` first. Confirm
that the current adapter changes `word` to `word-alt` before implementing.

## Files likely touched

- `apps/web/lib/theme/pierre-theme.ts`
- `apps/web/lib/theme/pierre-theme.test.ts`

## Dependencies

None. Run after Task 02 in the primary conversation.

## Risks

Pierre's public option setter replaces omitted values with defaults. Read
options inside the queued operation and keep using the public API.

## Parallelism

`sequential`

## Inputs

The rendering design's Pierre adapter and option-preservation contract;
installed Pierre 1.1.22 source and current adapter tests.

## Results

- Red: the real zero-worker Pierre manager reset `lineDiffType` from `word` to
  `word-alt` and reset the tokenization limit. The queued-options test also failed.
- Green: the adapter reads current render options inside the serialized operation
  and changes only the palette. Custom and built-in switches retain all four
  non-theme options; queued changes are read at execution time.
- All four Pierre tests, adapter tests and the existing slow-switch regression
  passed in the combined frontend run. Changed-file ESLint passed.
- The final combined verification is recorded in Task 04.
