---
created: 2026-10-08
status: done
requirements:
  - REQ-UI-CHAT-MOTION-003
system_design:
  - ../../specs/ui/system-design/chat-motion.md
legacy_specs: []
---

# Implementation Plan: Bounded Chat Scroll Settlement

## Overview

Stop smooth transcript following after the browser reaches an acceptable bottom
position. A fractional scroll position currently keeps the driver active forever.
One work order adds regression coverage and corrects the termination predicate.

This package follows investigation of
[issue 4100's latest report](https://github.com/kdlbs/kandev/issues/4100#issuecomment-6067024473).
It fixes a confirmed scroll-loop defect. It does not establish the cause of the
reported macOS WebKit memory growth. The issue must remain open for that diagnosis.
The issue is already assigned to the authenticated user, `carlosflorencio`.

## Requirement conformance

The active [chat-motion requirement](../../specs/ui/requirements/chat-motion.md)
already defines bounded settlement in AC-UI-CHAT-MOTION-003.1.
AC-UI-CHAT-MOTION-003.2 protects reader ownership and existing placement behavior.
AC-UI-CHAT-MOTION-003.4 requires bounded motion work and cancellation for hidden
or unmounted transcripts. No new requirement is necessary.

The owning [system design](../../specs/ui/system-design/chat-motion.md#bounded-settlement-with-browser-rounding)
now specifies termination with browser rounding. This local correction creates
no new ownership, persistence, security, or public API boundary. No ADR is needed.
The completed [chat-motion package](../chat-motion/plan.md) remains the historical
delivery record. This package owns the new regression and verification results.

## Evidence and confirmed root cause

The source at `6254b05eb0242b67900e160ff5b1d9acbb7962ad` and tag `v0.97.0`
contains the same exact-equality termination predicate in `createChatScrollMotion`.
The driver calculates an integer target from `scrollHeight - clientHeight`.
Browsers can return a fractional `scrollTop` after the write. When equality is
impossible, every frame reads geometry, writes the target, and queues another frame.

The [evidence and repeatable probe](evidence.md) uses the actual source in isolated
browser documents. Chromium retained a pending frame in 6 of 16 geometry cases
after 600 simulated frames. A temporary unit regression produced 600 geometry
reads and failed the expected settlement assertion. The existing 17 driver tests passed.

A candidate change evaluated only in memory settled all 16 Chromium cases and
all 16 Linux WebKit cases within 11 callbacks. Linux WebKit also passed all cases
with the original source. These results prove the fractional-position defect
and candidate behavior, not a macOS soak-test result or a memory-leak correction.

## Scope

### In scope

- Bounded settlement in `createChatScrollMotion`.
- Deterministic coverage for fractional, clamped, and subsequent-growth cases.
- Desktop and phone browser coverage through the existing chat-motion scenarios.
- Preservation of cancellation, reader intent, and deferred geometry reads.

### Out of scope

- Claims that this correction resolves all of issue 4100.
- WebKit stylesheet churn, retained animation objects, and code-server iframe retention.
- Desktop inspector controls, release packaging, and the already-fixed zombie cleanup.
- Store retention changes from PRs 4301 and 4302.
- New settings, runtime flags, localization keys, or telemetry.

## Technical approach

In `apps/web/components/task/chat/chat-scroll-motion.ts`, replace exact equality
with a 1 px tolerance and the current interpolation deadline. Preserve the
existing final target write, target reset, single-frame ownership, and retargeting.
When progress reaches 1, stop even if browser clamping prevents exact settlement.
Future content requests can retry from the current position.

Keep `use-chat-scroll-motion.ts` as the visibility and follow-policy owner.
Its existing `canFollow()` guard and effect cleanup already cancel hidden panels.
Do not add polling, synchronous reads in message commits, or another animation owner.

## ASCII UI preview

UI-01: Settled transcript, opened from a task session. The rendered controls and
composition stay the same. The behavior change is termination of idle follow work.

```text
Desktop task workbench             Phone task chat
+--------------------------+      +----------------------+
| Session tabs             |      | Task / session       |
| Transcript               |      | Transcript           |
|   latest message         |      |   latest message     |
|   [bottom within 2 px]    |      |   [bottom within 2 px]|
+--------------------------+      +----------------------+
| Composer                 |      | Composer + safe area |
+--------------------------+      +----------------------+

Before: a fractional bottom can keep the follow driver active.
After: the follow driver settles and waits for new content.
```

The transcript remains the single scroll owner. The composer stays outside it.
The nearest shipped exemplar is `task-chat-panel.tsx` and the phone full-height
chat surface. The mobile language's dense-viewer pattern applies. Entry points,
touch targets, dynamic viewport handling, safe areas, and navigation stay intact.
The same driver serves both views. Touch interruption remains authoritative.
The drawing is illustrative. Scroll ownership and bounded settlement are required
by AC-UI-CHAT-MOTION-003.1, .2, and .4.

## Tests

Extend `components/task/chat/chat-scroll-motion.test.ts`:

| Planned regression | Acceptance criteria |
| --- | --- |
| `settles when the browser cannot represent the exact bottom target` | AC-UI-CHAT-MOTION-003.1 |
| `stops after the interpolation deadline when writes remain clamped` | AC-UI-CHAT-MOTION-003.1, .4 |
| `restarts following after a rounded settlement and later growth` | AC-UI-CHAT-MOTION-003.1 |
| Existing retargeting, deferred reads, wheel, touch, and keyboard cases | AC-UI-CHAT-MOTION-003.1, .2 |
| Existing hidden/disposed driver tests in `use-chat-scroll-motion.test.tsx` | AC-UI-CHAT-MOTION-003.4 |

The first regression must fail against the original source before implementation.
Count geometry reads after settlement as well as pending callbacks. A position
assertion alone cannot detect this defect because the viewport already appears settled.

## E2E tests

Extend `apps/web/e2e/tests/chat/chat-motion-helpers.ts` and its existing desktop
`chat-motion.spec.ts` and phone `mobile-chat-motion.spec.ts` entry points.
Cover fractional geometry at noninteger zoom, streaming completion, later growth,
and preserved wheel/touch interruption. Use the actual active transcript from a
seeded task and current motion preference. Assert the final bottom tolerance.

Observe driver-attributable geometry reads or callbacks during a bounded idle
window after completion. Do not count global frames from unrelated status motion.
Install observation only in the disposable test page and restore it in cleanup.
Use the existing `dwell` helper with a negative-assertion reason for the idle window.
This scenario covers AC-UI-CHAT-MOTION-003.1, .2, and .4 on both projects.
The unit regression is the deterministic RED gate. Full browser checks require
a fresh production build through the managed runner.

## Work orders

- [x] [Task 01: Bound scroll settlement](task-01-bound-scroll-settlement.md)

Run sequentially in the primary conversation. There are no dependencies or
delegated tasks. The work order contains exact implementation verification commands.

## Verification results

- The new fractional, clamped, and later-growth unit regressions failed against
  the original exact-equality driver, then passed with the bounded settlement.
- The driver suite passed all 21 tests. The work-order unit command passed all
  59 chat scroll tests. Scoped ESLint and web typecheck passed.
- Chromium and mobile-chrome chat-motion scenarios each passed both tests after
  a fresh managed build. The scenario checked 1.125 CSS zoom, fractional
  `scrollTop`, stream completion, later growth, idle geometry reads, and the
  existing wheel/touch interruption paths.
- The Chromium idle-read regression observed 21 extra reads with the original
  driver and no extra reads with the fix.
- `python3 scripts/list-docs.py validate` passed (365 decisions, 1475 specifications).
- `python3 scripts/lint-spec-files.py --all`, public-doc validation, and
  `git diff --check` passed.
- The issue 4100 macOS long-uptime memory report remains unconfirmed and needs
  a separate investigation.

## Risks

- Early settlement must retain bottom-follow intent so later content restarts motion.
- A deadline must not prevent retargeting while content genuinely grows.
- Linux WebKit differs from the reporter's macOS WKWebView. The full incident remains unconfirmed.
- CPU work from a scroll loop does not prove stylesheet churn or animation retention.

## Documentation impact

Internal design and delivery documents change. Public documentation needs no
change because the accepted chat behavior, settings, and controls stay the same.
