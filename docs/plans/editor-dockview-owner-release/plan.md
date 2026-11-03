---
created: 2026-10-28
status: done
requirements:
  - REQ-UI-SESSION-REFRESH-EFFICIENCY-004
system_design:
  - ../../specs/ui/system-design/session-refresh-efficiency.md
legacy_specs: []
---

# Editor and Dockview owner release

This package records contribution 3 extracted from the reviewed dense-workbench
performance plan. The user authorized implementation and one focused PR on
current main. Source commits are `30e72e160e5a` and `f5c8fb8f6ab5`, retained at
`a659c453d3bcb67ca790a588f500f35b068dfd09`. Source worktrees remain read-only.

## Work order

- [Task 01: Release departed owners](task-01-release-departed-owners.md): done.

## Scope and verification

Balance editor user-subscription references and clear departed Dockview API
ownership. Preserve replacement owners, drafts, saved layouts and root user
subscriptions. The work order contains the exact checks and results.

Desktop retains the existing Dockview workbench. Phone retains the existing task
layout and task picker. Responsive transitions must release the desktop API and
preserve drafts. No markup, styling, animation, copy, navigation or touch-control
change is proposed, so no new UI composition preview is needed.

Unit regressions prove reference ownership and stale-cleanup isolation. Managed
Chromium and mobile-chrome tests prove navigation and draft preservation. Browser
runs and builds use the shared host validation lock. Historical counter and heap
measurements do not establish a broad latency improvement or heap leak fix.

No public documentation update is required: this repair enforces the existing
lifecycle contract without changing user instructions, APIs or configuration.
