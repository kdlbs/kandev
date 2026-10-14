---
status: active
system: tasks
created: 2026-10-12
owners:
  - kandev
---

# Terminal target lifecycle requirements

## Overview

Task environments own ordinary shell terminal targets. A browser can retain a
terminal pane after missing the event that removed its task and environment.
That pane must distinguish a removed target from a temporarily unavailable one.

## Requirements

### REQ-TASKS-TERMINAL-TARGET-001: Stop reconnecting to removed environments

**Intent:** A removed task environment does not leave a retained terminal
connecting forever or repeatedly requesting an impossible shell.

**User story:** As a user, I want a retained terminal to explain that it ended
when its environment no longer exists.

#### Acceptance criteria

- **AC-TASKS-TERMINAL-TARGET-001.1:** When the backend confirms an environment is
  missing, its terminal connection shall stop automatic reconnection.
- **AC-TASKS-TERMINAL-TARGET-001.2:** Desktop and phone terminal panes shall show
  the existing ended state and remove connecting feedback for that target.
- **AC-TASKS-TERMINAL-TARGET-001.3:** A temporary lookup or runtime-readiness
  failure shall retain the existing retry behavior.
- **AC-TASKS-TERMINAL-TARGET-001.4:** An unavailable target shall not prevent
  connecting a different environment. Late callbacks shall not retire a
  replacement connection.
- **AC-TASKS-TERMINAL-TARGET-001.5:** Missing-target handling shall preserve
  authentication and origin checks and shall not recreate deleted resources.

## Out of scope

- Changing agent prompt recovery, clearing uncertainty, or replaying prompts.
- Recreating deleted task environments or terminal processes.
- Changing terminal layout, touch controls, persistence, or task navigation.
