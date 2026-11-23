---
id: "01-reveal-first-range"
title: "Reveal the first Monaco walkthrough range"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-WALKTHROUGH-EDITOR-REVEAL-001
acceptance_criteria:
  - AC-UI-WALKTHROUGH-EDITOR-REVEAL-001.1
  - AC-UI-WALKTHROUGH-EDITOR-REVEAL-001.2
system_design:
  - ../../specs/ui/system-design/walkthrough-editor-reveal.md
---

# Task 01. Reveal the First Monaco Walkthrough Range

## Summary

Make the initial Monaco reveal use the same ready editor and model as its
highlight. Verify the reported behavior before changing production code.

## Scope

- Own `apps/web/components/editors/monaco/use-monaco-walkthrough-range.ts` and
  its adjacent test. Trace the initial cursor request in
  `apps/web/lib/diff/walkthrough-reveal.ts` to identify reveal order.
- Add a targeted browser case in
  `apps/web/e2e/tests/review/walkthrough.spec.ts`. Use a long file and a fresh
  Monaco tab. Extend the mock-agent walkthrough fixture in
  `apps/backend/cmd/mock-agent/scenarios.go` if needed to supply this anchor.
- Start with a failing regression for the missed reveal. Confirm whether
  model readiness, layout, or view restoration is responsible before choosing
  the smallest correction.
- Preserve the editor provider, walkthrough card, and phone sheet behavior.

## Acceptance

- The first referenced range reaches the Monaco viewport on open. The test
  checks containment within the editor before it clicks a step control.
- Next and Previous still reveal their ranges. A completed reveal does not
  repeat when the reader scrolls or the editor resizes.

## ASCII UI preview

**UI-01. Desktop walkthrough launcher, first step open**

```text
+ Editor ------------------+
| 152  Context             |
| 153  Highlighted line    |
| 154  Highlighted line    |
|             [Step 1]    |
+-------------------------+
```

The existing editor scrolls to the highlighted range. The user can continue
with the existing step controls. See the [full preview](plan.md#ascii-ui-preview).

## Verification

From the repository root:

```bash
pnpm --dir apps install --frozen-lockfile
pnpm --dir apps/web test components/editors/monaco/use-monaco-walkthrough-range.test.ts
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --project chromium tests/review/walkthrough.spec.ts -- --grep 'reveals the first Monaco range on open'
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome tests/review/mobile-walkthrough.spec.ts -- --grep 'opens the walkthrough as a bottom-sheet panel'
```

## Dependencies and risks

No work-order dependencies. Use the active editor's model identity and viewport.
A mounted editor can still lack a usable layout. Avoid delaying all file
navigation or recentering on unrelated layout changes.

## Results

- The delayed-layout regression failed on the original code. Monaco remained
  at scroll position zero after its first usable layout.
- After the fix, 11 hook tests passed. Both desktop Chromium tests and the
  phone smoke test passed.
- Web type checking, focused lint, and whitespace checks passed.
