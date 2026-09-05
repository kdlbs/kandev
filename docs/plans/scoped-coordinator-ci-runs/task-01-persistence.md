---
id: "01-persistence"
title: "Persist scoped grants and idempotent CI requests"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/scoped-coordinator-ci-runs.md"
---

# Task 01: Persist scoped grants and idempotent CI requests

Add replayable grant, request, and audit tables. Prove actor-key replay,
semantic source-attempt uniqueness, concurrent claims, provider-start markers,
redacted evidence, and workspace cleanup using SQLite tests under `-race`.
