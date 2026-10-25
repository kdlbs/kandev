---
created: 2026-10-25
status: in-progress
requirements:
  - REQ-AGENTS-PI-REMOTE-AUTH-001
  - REQ-AGENTS-PI-REMOTE-AUTH-002
system_design:
  - ../../specs/agents/system-design/pi-remote-auth.md
---

# Implementation Plan: Pi Remote Credentials

## Overview

Declare Pi's credential file, API-key variables and configuration bundle so
the Remote Credentials card offers them on every remote executor, and forward
the one missing variable over SSH.

## Work orders

- [Task 01: Declare Pi remote credentials and configuration](task-01-declare-pi-remote-auth.md)

## Verification

- Unit: `go test ./internal/agent/agents -run PiACP` and
  `go test ./internal/agent/runtime/lifecycle -run SSHRemoteAgentEnv`.
- Web: `pnpm run i18n:check`, typecheck and the profile-edit component tests.
