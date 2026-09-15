---
id: "01-persistence"
title: "Persist scoped grants and idempotent CI requests"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-SCOPED-CI-RUNS-001
acceptance_criteria:
  - AC-INTEGRATIONS-SCOPED-CI-RUNS-001.13
  - AC-INTEGRATIONS-SCOPED-CI-RUNS-001.16
system_design:
  - ../../specs/integrations/system-design/scoped-coordinator-ci-runs.md
---

# Task 01: Persist scoped grants and idempotent CI requests

Add replayable grant, request, and audit tables. Prove actor-key replay,
semantic source-attempt uniqueness, concurrent claims, provider-start markers,
redacted evidence, and workspace cleanup using SQLite tests under `-race`.
