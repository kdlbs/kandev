---
id: "01-scope-snapshot"
title: "Scope the PR snapshot and check its head"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-PR-STATE-001
system_design:
  - ../../specs/integrations/system-design/pr-state-snapshots.md
acceptance_criteria:
  - AC-INTEGRATIONS-PR-STATE-001.1
  - AC-INTEGRATIONS-PR-STATE-001.2
---

# Scope the PR snapshot and check its head

## Files likely touched

- `scripts/pr-state` selects the repository and compares head commits.
- `scripts/pr-state.test.sh` covers explicit repository selection and stable and changing heads.

## Verification

Run `bash scripts/pr-state.test.sh`. The suite passed. A read-only replay against
PR #2308 returned matching head commits in a scratch repository without remotes.
