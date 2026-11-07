---
id: "02-canonical-workspace"
title: "Resolve the current task workspace for local review"
status: in_progress
wave: 2
depends_on: ["01-local-source-fallback"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-001
acceptance_criteria:
  - AC-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-001.1
  - AC-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-001.2
  - AC-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-001.3
  - AC-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-001.4
system_design:
  - ../../specs/agents/system-design/native-code-review-recovery.md
---

# Task 02: Current task workspace

When a session has a durable environment, resolve its current admitted
workspace before a local review reads Git. Preserve task/session identity,
the existing local-only restriction, repository-relative panel keys and, when
no inspector is wired, the legacy fallback for unbound sessions. The operation
only inspects the existing workspace; it does not repair, start, move or create one.

Entry points: `lifecycle/session_workspace_inspection.go`,
`backendapp/review_local_workspace.go`, `backendapp/review_local_source.go`,
and the existing review wiring. Reuse the lifecycle workspace admission and
checkout validation instead of accepting a caller-provided path.
