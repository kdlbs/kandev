---
id: "01-reviewer-exclusion"
title: "Exclude the writer from workflow review"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-NATIVE-CODE-REVIEW-002
acceptance_criteria:
  - AC-AGENTS-NATIVE-CODE-REVIEW-002.1
  - AC-AGENTS-NATIVE-CODE-REVIEW-002.2
  - AC-AGENTS-NATIVE-CODE-REVIEW-002.3
  - AC-AGENTS-NATIVE-CODE-REVIEW-002.4
  - AC-AGENTS-NATIVE-CODE-REVIEW-002.5
  - AC-AGENTS-NATIVE-CODE-REVIEW-002.6
system_design:
  - ../../specs/agents/system-design/native-code-review-reviewer-exclusion.md
---

# Task 01: Distinct workflow reviewer

Capture the actual writer at step entry, exclude its profile and agent/model
from the review selection, and repeat the check at final run admission and
fallback. Preserve candidate ordering, ordinary review behavior and the existing
run owner. Accept the native action in the workflow YAML loader without changing
the shipped default workflow.

Validate using focused existing checks and disposable source tests. Do not start
a provider or add a persistent test suite as part of this port.
