---
status: active
system: cli
created: 2026-09-03
owners:
  - kandev
---

# Passthrough Launch Defaults Requirements

## Overview

CLI passthrough gives users the native interactive output of an agent. Built-in
launch arguments must preserve that standard output unless a profile selects a
different mode.

## Requirements

### REQ-CLI-PASSTHROUGH-LAUNCH-001: Keep diagnostic output optional

**Intent:** A user can operate Claude CLI passthrough without forced diagnostic
output. The user can enable diagnostic output for a selected profile.

#### Acceptance criteria

- **AC-CLI-PASSTHROUGH-LAUNCH-001.1:** When Kandev starts built-in Claude CLI passthrough without an enabled `--verbose` profile flag, the launched command shall omit `--verbose`.
- **AC-CLI-PASSTHROUGH-LAUNCH-001.2:** When the Claude profile contains an enabled `--verbose` CLI flag, the launched passthrough command shall include `--verbose`.
- **AC-CLI-PASSTHROUGH-LAUNCH-001.3:** The quiet default shall preserve model selection, permission flags, session resume, prompt delivery, and MCP configuration.
- **AC-CLI-PASSTHROUGH-LAUNCH-001.4:** Existing profiles shall require no data migration. A user can add `--verbose` through the existing CLI-flags field.

## Out of scope

- A dedicated verbose-output setting.
- Changes to Claude ACP execution.
- Changes to the launch defaults of other agents.
- Changes to Claude Code output rendering.
