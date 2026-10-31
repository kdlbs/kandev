---
id: "01-retain-interaction-fixes"
title: "Retain native scrolling, split restoration, and plan link fixes"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TRANSCRIPT-AUTO-SCROLL-001
  - REQ-UI-SESSION-REFRESH-EFFICIENCY-004
acceptance_criteria:
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.20
  - AC-UI-SESSION-REFRESH-EFFICIENCY-004.3
system_design:
  - ../../specs/ui/system-design/transcript-auto-scroll.md
  - ../../specs/ui/system-design/session-refresh-efficiency.md
---

# Retain interaction fixes

## Outcome and scope

Deliver the correctness scope in the [plan](plan.md). Own the layout comparator,
native-scroll producer, frame coalescer, both plan renderers, adjacent tests,
and focused E2E coverage. Source commits: `72085e06b490`, `7a9840211131`,
`138a567d45b8`, `2f3c343e04bd`, `4f078c130343`, and `c5ed714af358`.
No other contribution is a dependency. Source worktrees remain read-only.

## Acceptance

1. Two tasks with identical panels restore their distinct split trees and drafts.
2. Upward intent survives queued follow events. A measured return to bottom
   resumes following. Detached captures cannot publish obsolete positions.
3. Plan links remain editable without opening tabs, and link/underline content
   survives serialization on desktop and phone.

## UI preview and mobile contract

Use the [plan's UI preview and mobile contract](plan.md#ui-preview).
The existing controls and styles remain. Saved desktop split trees differ per
task. Phone Chat and Plan retain their focused scroll surfaces.

## Verification

Run from the repository root. Serialize browser builds and runs with
`flock /tmp/kandev-contribution-validation-5936a872.lock`.

```bash
pnpm --dir apps/web exec vitest run lib/state/layout-manager components/task/chat/message-list-native.test.tsx components/task/chat/transcript-auto-scroll.test.ts components/task/chat/chat-scroll-motion.test.ts lib/state/slices/ui/transcript-auto-scroll-actions.test.ts components/editors/tiptap/tiptap-plan-editor.test.tsx
pnpm --dir apps/web run typecheck
pnpm --dir apps/web exec eslint --max-warnings 0 <changed-ts-files>
pnpm --dir apps/web e2e:run --host --project chromium tests/layout/saved-layout-arrangement.spec.ts tests/chat/native-upward-intent.spec.ts tests/task/plan-link-editing.spec.ts tests/task/plan-table-resize.spec.ts tests/task/plan-checkpointing.spec.ts tests/search/plan-search.spec.ts
pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome tests/task/mobile-plan-link-editing.spec.ts tests/task/mobile-plan-formatting-toolbar.spec.ts tests/task/mobile-plan-restore.spec.ts tests/task/mobile-plan-table-resize.spec.ts tests/chat/mobile-auto-scroll-toggle.spec.ts tests/task/mobile-task-navigation-responsiveness.spec.ts
```

## Results

Implementation and local verification are complete. PR checks and automated review are tracked in the PR.

- Initial RED runs reproduced four layout mismatches, duplicate plan marks,
  unwanted link navigation, wheel-follow failures, and obsolete frame capture.
- Initial GREEN: 286 tests pass across 17 files. Review remediation adds
  six near-edge cases; all 292 focused tests pass with the final production patch.
- Nested keyboard regression: three cases fail with the new guard removed.
  The guard preserves upward intent for textarea, contenteditable, and prevented End.
- Strict changed-file lint and documentation validation pass.
- Default-heap typecheck exits 134 with V8 heap exhaustion. The serialized
  retry passes with `NODE_OPTIONS=--max-old-space-size=6144`.
- Fresh backend, web assets, and E2E fixture plugin build successfully.
  The first browser launch rejects port offset 41 (valid range 0-29).
  It executes no tests. The corrected launch uses offset 24.
- The next attempt fails before browser startup because Chromium cannot load
  `libglib-2.0.so.0`. It is interrupted after repeated launch failures (exit 130).
  The retry supplies the source investigation's Nix library paths, verified
  with `ldd`, and retains headless managed execution. Inherited GitLab host/token,
  bundle, and launcher settings are removed only from the disposable environment.
- Browser discovery finds all five new desktop/phone cases. Managed browser
  checks are complete on desktop: all 21 pass. All 12 phone checks also pass.
- Fresh desktop ledgers record 0 px held-reading movement and a 1 px final
  bottom gap after native PageDown followed by growth.
- Final desktop capture reruns all four new cases successfully. Six compressed
  desktop/phone screenshots are inspected. Terminal contents are hidden to omit
  host shell identity. The interaction assertions remain unchanged.

Current-main content-reflow safeguards are retained. The layout E2E uses the
existing navigation fixture without importing the research benchmark helpers.
Browser replay preserves the source event ordering and assertion bounds.
Unit regressions provide the initial RED boundary for those replay scenarios.

## Review remediation

A review identified stale upward intent after a return from a 1 px gap.
Five unit regressions reproduce failures for downward keys, touch, and explicit
latest navigation. End already passes. Fresh downward intent and latest
ownership now release the stale origin. Nested controls keep their key guard.
The six cases pass with the fix; changed-file lint and the 6 GiB typecheck pass.

The browser replay now includes both 160 px and 1 px returns. It focuses the
transcript before PageDown and waits for native movement and scrollend before
appending output. A coordinate click in the 1 px case selected a Copy button;
the resulting nested-key behavior was not a transcript return. Temporary event
traces established that target and were removed. Outcome bounds stay unchanged.
All three desktop scroll checks pass after focusing the transcript. An earlier
full batch had one fixture startup failure (`steps` was null before the test);
the single rerun passes. All seven phone auto-scroll checks pass. Refreshed
screenshots and exact-head CI/review remain delivery steps tracked in the PR.

## Documentation impact

Public commands, settings, copy, and interaction controls do not change.
The existing `docs/public/sessions-and-review.md` describes independent
transcripts, reading without forced following, retained drafts, and saved
workbench layouts. Internal scroll design records the cancellation invariant.
