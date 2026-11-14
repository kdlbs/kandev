---
id: "01-motion-eligibility"
title: "Retained status motion eligibility"
status: done
wave: 1
plan: "plan.md"
depends_on: []
requirements:
  - REQ-UI-PERSISTENT-STATUS-MOTION-004
  - REQ-UI-PERSISTENT-STATUS-MOTION-006
system_design:
  - ../../specs/ui/system-design/persistent-status-motion.md
acceptance_criteria:
  - AC-UI-PERSISTENT-STATUS-MOTION-004.1
  - AC-UI-PERSISTENT-STATUS-MOTION-004.2
  - AC-UI-PERSISTENT-STATUS-MOTION-004.3
  - AC-UI-PERSISTENT-STATUS-MOTION-004.4
  - AC-UI-PERSISTENT-STATUS-MOTION-004.5
  - AC-UI-PERSISTENT-STATUS-MOTION-006.1
  - AC-UI-PERSISTENT-STATUS-MOTION-006.2
  - AC-UI-PERSISTENT-STATUS-MOTION-006.3
---

# Retained status motion eligibility

## Outcome and scope

Port retained changes from `e7180040058d`, `d7efb570d286`, `ef17d222ae26`, and
`7808415cf2e3`. The shared spinner, grid, and persistent rotation respect runtime
reduced motion. Threads status uses geometric owner visibility independently
of preload, detail activation, and focus.

Owned production files are the shared compositor-spin, spinner, and
persistent-motion-visibility components, GridSpinner, scoped globals.css rules,
and Threads board, column, activation, header, and session-status components.
Include focused regressions and the persistent-status-motion contracts.
Exclude benchmark instrumentation and unrelated source-branch changes.

## Acceptance

1. Reduced motion leaves visible, static status in native and CSS fallback paths.
2. Hidden targets pause. Visible unfocused targets remain eligible. Hidden status updates settle normally.
3. Repeated preference and visibility changes release animation handles, observers, and listeners.

## ASCII UI preview

UI-01: Existing status presentation, desktop and phone.

```text
Desktop Threads: [status icon] Task title       [session picker]
Phone Threads:   [task navigation] [status icon] Status label
Task composer:  [nine-cell grid]                 [Stop]
Plugin action:  [loading icon] Action label
```

The static reduced-motion pose preserves the same geometry and labels.
Normal motion retains its timing. The phone exemplar is the existing mobile
Threads header and Quick Chat activity indicator. No control, touch target,
scroll owner, safe-area rule, or navigation changes. Spacing is illustrative.
The rendered checks cover AC-UI-PERSISTENT-STATUS-MOTION-004.4 and -006.1.

## Verification

From `apps/web`:

```bash
pnpm exec vitest run lib/ui/persistent-motion-preference.test.tsx lib/ui/persistent-motion-visibility.test.tsx lib/ui/compositor-spin.test.tsx lib/ui/compositor-pulse.test.tsx components/threads/thread-status-visibility.test.tsx components/threads/thread-column-activation.test.tsx components/grid-spinner.test.tsx
pnpm e2e:run --project chromium tests/chat/persistent-animation-motion.spec.ts tests/chat/quick-chat-idle-dot.spec.ts tests/task/threads-status-motion.spec.ts tests/plugins/plugin-action-motion.spec.ts
pnpm e2e:run --project mobile-chrome tests/chat/mobile-persistent-animation-motion.spec.ts tests/chat/mobile-quick-chat-idle-dot.spec.ts tests/task/mobile-threads-status-motion.spec.ts tests/plugins/mobile-plugin-action-motion.spec.ts
NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck
```

Serialize browser/build commands with the shared host lock named in the plan.
Run strict lint for changed web/shared files, spec validation, and public-doc
validation. Capture desktop and phone states using isolated test data.

## Results

- Current-main baseline: `d17d84a8084b15ddf1df10ff465a51d27c780e64`.
- Unit RED: 14 failures and 29 passes across five files before production changes.
- Fast-forwarded to `aa229b60dd9281043f7638767777222f5723f710` before final browser validation. Its database-maintenance changes do not overlap this work.
- Initial unit GREEN: all 43 focused tests and ten existing compositor tests passed.
- Review fix: grid setup now changes only `animation-name`, preserving the visibility controller's CSS pause. A real Chromium regression failed before the fix when the fallback mounted in a hidden document. The desktop and phone fallback cases now pass, including offscreen preference changes. All 54 motion unit tests and changed-file lint pass.
- Review documentation fixes: requirements 004 and 006 and the visibility design are accepted; requirement 005 remains draft. The typecheck command includes the tested heap setting.
- CI documentation coverage: the trusted evaluator reproduced a missing `wave` field. Adding `wave: 1` passes the evaluator for the exact PR file set, plus the specification catalog and lint checks.
- Strict web lint passes. Shared lint passes with the web-only literal-string rule excluded for the existing shared Spinner label.
- Web typecheck passes with `NODE_OPTIONS=--max-old-space-size=8192`. The default 4 GB Node heap exhausted before diagnostics.
- Specification catalog/lint and public-doc validation pass, including 63 public-doc validator tests.
- Browser RED against the frozen pre-fix assets reproduced both reduced-motion failures: two plugin indicators kept running and the Threads icon retained one running animation. Initial attempts failed before assertions because of inherited bind configuration and missing browser library paths.
- Fresh production build: six desktop/tablet and six phone browser checks pass. All six final capture checks pass after hiding terminal content during captures. Desktop and phone images were inspected for visible status, preserved geometry, and synthetic data.
- Browser commands used the shared lock, explicit loopback binding, and the existing Nix browser library path. Only the intentional frozen-baseline run skipped artifact freshness.
- Delivery continues through the contribution PR. CI and review status belong to the current PR head.

## Historical performance evidence

The [owned Threads evidence](https://github.com/kdlbs/kandev/blob/a659c453d3bcb67ca790a588f500f35b068dfd09/docs/plans/dense-workbench-performance/evidence/owned-thread-motion-six-pair-1x.json)
records 12 verified Chromium samples and 21,600 ordered updates in the scoped
30-busy-header workload. Normal idle main-thread CPU fell about 70.7% there.
Default six-header streaming results were essentially neutral. These are
historical builds, not measurements of this extraction. No general page-speed,
Firefox/M2, task-switch target, or heap-leak claim follows.
