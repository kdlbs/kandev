---
id: "02-provider"
title: "Add server-owned GitHub Actions operations"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/scoped-coordinator-ci-runs.md"
---

# Task 02: Add server-owned GitHub Actions operations

Require Actions write on repository-scoped installation tokens and implement
typed reads, failed-job rerun, dispatch, reconciliation, and provider failure
classification. Tests use an HTTP fixture and assert authorization headers and
tokens never escape receipts or errors.
