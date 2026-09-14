---
id: "02-provider"
title: "Add server-owned GitHub Actions operations"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-SCOPED-CI-RUNS-001
acceptance_criteria:
  - AC-INTEGRATIONS-SCOPED-CI-RUNS-001.5
  - AC-INTEGRATIONS-SCOPED-CI-RUNS-001.7
  - AC-INTEGRATIONS-SCOPED-CI-RUNS-001.8
system_design:
  - ../../specs/integrations/system-design/scoped-coordinator-ci-runs.md
---

# Task 02: Add server-owned GitHub Actions operations

Require Actions write on repository-scoped installation tokens and implement
typed reads, failed-job rerun, dispatch, reconciliation, and provider failure
classification. Tests use an HTTP fixture and assert authorization headers and
tokens never escape receipts or errors.
