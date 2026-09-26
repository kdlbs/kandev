---
created: 2026-09-26
status: done
requirements:
  - REQ-AGENTS-RUNTIME-UPDATES-001
system_design:
  - ../../specs/agents/system-design/runtime-updates-01.md
legacy_specs: []
---

# Plan: Refresh stable managed runtime pins

## Scope

Refresh the central managed npm runtime catalogue from stable npm `latest`
metadata. The update changes reviewed defaults only; it does not install or
activate a runtime.

## Work orders

- [x] [Task 01: Refresh stable managed runtime pins](task-01-refresh-stable-pins.md)
