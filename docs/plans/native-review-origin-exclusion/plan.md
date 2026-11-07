---
created: 2026-11-07
status: in_progress
requirements:
  - REQ-AGENTS-NATIVE-CODE-REVIEW-002
system_design:
  - ../../specs/agents/system-design/native-code-review-reviewer-exclusion.md
legacy_specs: []
---

# Native review origin exclusion

Port the existing writer-identity and reviewer-exclusion behavior onto the
native review ownership and dynamic-candidate stack. Retain configured order,
ordinary reviews, durable attribution and the workflow transition on refusal.

- [Implementation and validation](task-01-reviewer-exclusion.md)
