---
created: 2026-10-28
status: done
requirements:
  - REQ-UI-PERSISTENT-STATUS-MOTION-004
  - REQ-UI-PERSISTENT-STATUS-MOTION-006
system_design:
  - ../../specs/ui/system-design/persistent-status-motion.md
---

# Status motion eligibility extraction

Extract contribution 5 from the reviewed dense-workbench performance package
at source commit `a659c453d3bcb67ca790a588f500f35b068dfd09` onto current main.
The user authorized implementation and one focused PR. The source package and
its rejected experiments remain read-only historical evidence.

## Work order

| Work order | Status | Dependencies |
| --- | --- | --- |
| [01: Retained motion fixes](task-01-motion-eligibility.md) | done | None |

## Scope and validation

Implement the retained reduced-motion and hidden-owner fixes only. Keep the
existing direct SVG target, normal colors, timing, and status semantics.
No wrapper migration, observer pool, global motion disable, or Firefox hint.

The work order owns unit, desktop/phone browser, lint, and type checks.
All browser/build work uses `/tmp/kandev-contribution-validation-5936a872.lock`.
The main risk is source drift: port scoped changes without replacing newer UI
or navigation behavior. Historical Chromium results do not establish general
page speed, Firefox/M2 gains, or a heap leak fix.

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
