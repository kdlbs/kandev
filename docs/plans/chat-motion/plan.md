---
created: 2026-09-15
status: implemented
requirements:
  - REQ-UI-CHAT-MOTION-001
  - REQ-UI-CHAT-MOTION-002
  - REQ-UI-CHAT-MOTION-003
system_design:
  - ../../specs/ui/system-design/chat-motion.md
legacy_specs: []
---

# Implementation Plan: Chat Motion

## Overview

Deliver subtle chat motion with a default-on, per-device Appearance setting.
[Requirements](../../specs/ui/requirements/chat-motion.md) and
[system design](../../specs/ui/system-design/chat-motion.md) own the contract.
Implement text/settings first, then integrate scrolling with its existing
placement protections. Implementation was authorized in the subsequent user turn. Both work orders
are implemented and verified.

## Scope

In scope: live prose suffixes, new transcript rows, effective reduced-motion
policy, Appearance Save/Reset, smooth following and explicit chat navigation,
localized copy, desktop/mobile coverage and public settings documentation.

Out of scope: terminal/global motion, rich-output internal animation changes,
message delivery changes, backend persistence, speed settings, exit animations.
No subagents, commit, push, or PR are authorized by this package.

## Technical approach

Follow the design's existing rich-output-motion storage/actions pattern,
Appearance draft transaction, optional chat-only Markdown text-node transform,
and native transcript follow policy. New files named in work orders are proposed
implementation targets; existing files were inspected. No new package dependency
is planned. Reconcile the existing transcript pinning wording in work order 02
so live smoothing is a bounded form of following, while restoration stays instant.

## ASCII UI preview

### UI-01: Appearance, enabled and disabled (desktop and phone)

Entry: Settings > Preferences > Appearance. Existing navigation is retained.

```text
Appearance
  Rich output animations                [on]
  Chat animations                       [on]
  Animate incoming text, new chat items,
  and scrolling on this device.
  Respects reduced motion.
                              [Reset] [Save]

Saved off: Chat animations               [off]
OS reduced motion: saved switch stays on;
                   effective motion is off.
```

Both viewports retain this control order. Phone help text wraps below the
label; the switch has a 44 px hit area. Existing settings page owns scrolling
and Save/Reset placement. Spacing/copy are illustrative; control semantics,
localization, hierarchy, and touch reachability are required.

### UI-02: Live chat (shared content order, existing phone full-height layout)

```text
[Session header / navigation]                 fixed
[Existing transcript text]                    scrolls
[Assistant: existing text + incoming suffix]   suffix fades
[New tool row]                                enters once
[Composer and existing navigation controls]    outside scroller
```

Phone shows one full-height Chat surface; desktop retains its Dockview panel.
Only transcript content scrolls; composer and safe-area geometry are unchanged.
History/loading restoration and motion-off states use the same layout with
immediate content. No new empty/error state is introduced. UI-01 maps to
AC-UI-CHAT-MOTION-002.1 through .4; UI-02 maps to the 001 and 003 criteria.
The work-order E2E suites prove the rendered controls and motion states.

## Work orders

| Wave | Work order | Dependencies | Status |
| --- | --- | --- | --- |
| 1 | [01: Text motion and appearance](task-01-text-and-preference.md) | None | done |
| 2 | [02: Smooth scrolling](task-02-smooth-scrolling.md) | 01 | done |

Run sequentially: shared motion policy, transcript integration, and tests overlap.
Estimated implementation: 1 to 2 working days including browser verification;
Markdown reconciliation and scroll cancellation are the main uncertainty.

## Verification strategy

Each work order contains exact targeted unit and managed Playwright commands.
Use TDD for implementation; install workspace dependencies
once if absent. Browser suites build their own fresh production assets.
Record actual command results in each work order before marking it done.

Design gates (run from repository root):

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/chat-motion
git status --short -- docs/plans/chat-motion
```

## Risks and decisions

Inline Markdown structure can change mid-stream: static fallback is required
when append mapping is ambiguous. Grouped rows need child identity tracking.
Scroll animation must preserve logical follow intent without ignoring manual
scroll input. Existing synchronous-placement assertions remain valid for motion
off/restoration; add deterministic bounded-settle coverage for motion on.

Per-device persistence and OS precedence are routine design choices based on
the adjacent rich-output preference, not an account setting or release flag.
No material product questions remain open. Public documentation is updated
with implementation, per docs-maintainer.

## Results

Implementation authorized and completed in the primary session on 2026-09-15.
Both work orders are complete.

- Combined targeted unit run: 198 tests across 18 files passed.
- Final lint/selection/comment rerun: 94 tests across 5 files passed.
- Cached-history anchoring fix: 63 native-scroll/lifecycle tests passed.
- TypeScript, changed-file ESLint, translations, and production Vite build passed.
- Desktop suite: 22 passed initially; motion-test readiness was corrected and
  the final-build motion/settings and restoration cases passed. The hover test
  now waits for the pinned bar entrance to settle and passed three consecutive
  final-build runs. All 24 targeted desktop cases are covered by passing runs.
- Phone suite on the final build: all 9 passed, including cached history during
  refresh, touch interruption, motion setting persistence, and navigation.
- Desktop and phone browser traces observe actual opacity and intermediate
  scroll positions; unit clocks prove bounded runs and no idle frame loop.
- Public docs: 46 pages validated; specification catalog and 36 linter tests passed.
- Browser tests used the managed single-worker runner and freshly built assets.
  Local socket access required an approved sandbox escalation. The first native
  build used `GOCACHE=/tmp/kandev-chat-motion-go-cache`; later runs reused those
  backend artifacts with `--no-build` after rebuilding the changed web bundle.
- No commit, push, or PR was performed.
