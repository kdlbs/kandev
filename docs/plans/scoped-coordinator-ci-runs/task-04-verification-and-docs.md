---
id: "04-verification-and-docs"
title: "Verify fixture policy and document rollout"
status: done
wave: 3
depends_on: ["03-policy-and-mcp"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-SCOPED-CI-RUNS-003
  - REQ-INTEGRATIONS-SCOPED-CI-RUNS-005
acceptance_criteria:
  - AC-INTEGRATIONS-SCOPED-CI-RUNS-003.2
  - AC-INTEGRATIONS-SCOPED-CI-RUNS-003.5
  - AC-INTEGRATIONS-SCOPED-CI-RUNS-005.1
system_design:
  - ../../specs/integrations/system-design/scoped-coordinator-ci-runs.md
---

# Task 04: Verify fixture policy and document rollout

Cover rerun, dispatch, fork, merge evidence, provider failures, ambiguity,
audit redaction, and composition. Document App permissions, MCP boundaries, and
the two reviewed live consumers without executing either during development.
