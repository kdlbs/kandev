---
id: "03-observer-batches"
title: "Accept queued chart visibility entries"
status: done
wave: 3
depends_on: ["01-defer-and-stabilize-charts"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-001
acceptance_criteria:
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.7
system_design:
  - ../../specs/agents/system-design/agent-rich-output.md
---

# Task 03: Accept queued chart visibility entries

## Behavior

An intersection observer can deliver multiple entries for one plot in a batch.
The first entry can show an offscreen plot before a later entry shows it onscreen.
The callback must inspect the full batch so the plot does not stay empty.
A visible tab mounts an eligible plot once. Background tabs still defer plots.

## Validation

The queued offscreen/onscreen unit regression failed before the correction.
All eight chart scheduling tests pass after the correction.
The original desktop browser case reproduced the missing SVG in one of six attempts.
A fresh production build passed. Six desktop browser repetitions passed with retries disabled.
Six mobile browser repetitions also passed with retries disabled.
Strict lint, typecheck, and specification lint passed. Hosted validation remains pending.
