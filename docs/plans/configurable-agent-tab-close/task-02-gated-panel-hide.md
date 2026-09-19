---
id: "02-gated-panel-hide"
title: "Gate desktop panel hiding behind the preference"
status: done
wave: 2
depends_on:
  - "01-portable-close-preference"
plan: "plan.md"
requirements:
  - REQ-UI-AGENT-TAB-CLOSE-BEHAVIOR-001
acceptance_criteria:
  - AC-UI-AGENT-TAB-CLOSE-BEHAVIOR-001.3
  - AC-UI-AGENT-TAB-CLOSE-BEHAVIOR-001.4
  - AC-UI-AGENT-TAB-CLOSE-BEHAVIOR-001.5
  - AC-UI-AGENT-TAB-CLOSE-BEHAVIOR-001.6
  - AC-UI-AGENT-TAB-CLOSE-BEHAVIOR-001.7
  - AC-UI-AGENT-TAB-CLOSE-BEHAVIOR-001.8
system_design:
  - ../../specs/ui/system-design/agent-tab-close-behavior.md
---

# Task 02: Gated panel hide

Keep confirmed deletion as the default and use panel-local hiding only after the opt-in preference is saved.
