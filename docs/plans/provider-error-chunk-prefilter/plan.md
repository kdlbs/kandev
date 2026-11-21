---
created: 2026-11-14
status: implemented
requirements:
  - REQ-AGENTS-PROVIDER-ERROR-PREFILTER-001
system_design:
  - ../../specs/agents/system-design/provider-error-chunk-prefilter.md
legacy_specs: []
---

# Implementation Plan: Provider Error Classification Pre-Check on Message Chunks

## Overview

The ACP adapter runs the full provider-error rule engine on every message
chunk to set the diagnostic-candidate marker. One work order adds a
conservative pre-check derived from the rule catalogue and gates that call.

## Work orders

- [Task 01: Gate per-chunk classification](task-01-gate-chunk-classification.md)
