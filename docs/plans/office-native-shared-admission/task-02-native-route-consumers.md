---
id: "02-native-route-consumers"
title: "Bind native utility and review runs before selection"
status: in_progress
wave: 2
depends_on: ["01-office-native-admission"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-SESSION-CEILING-003
acceptance_criteria:
  - AC-AGENTS-SESSION-CEILING-003.1
  - AC-AGENTS-SESSION-CEILING-003.3
  - AC-AGENTS-SESSION-CEILING-003.4
system_design:
  - ../../specs/agents/system-design/session-concurrency-ceiling.md
---

# Task 02: Native utility and review route owners

Create the durable utility call or review run before dynamic candidate
selection, bind the exact owner and candidate, and use the existing native
admission owner. Failed or unverified attempts retain the appropriate owner;
successful settlement uses the existing child identity. Preserve the native
route alias on refusal and transfer it only to the matching inference child.
If capacity policy changes during that transfer, remove the same alias on
success and restore it on failure. Dynamic routing off keeps its existing path.

Entry points: `backendapp/review_candidate_attribution.go`,
`backendapp/review_route_owner.go`, `backendapp/review_wiring.go`,
`orchestrator/native_route_capacity.go`, `review/admission_wait.go`,
`review/resolver.go`, `review/run_owner.go`, `review/runner.go`,
`task/repository/sqlite/review_owner.go`, `task/service/review_owner.go`,
`utility/controller/controller.go`, `utility/controller/prompt_execution.go`,
`utility/handlers/handlers.go`, `utility/service/call_execution.go`,
`utility/service/profile_deadline.go`, `utility/service/profile_execution.go`,
`utility/service/profile_success.go`, `utility/service/prompt_owner.go`, and
`utility/service/service.go`.

The original source and its review are historical evidence for that source.
Integrated verification and its limits are recorded with the stacked draft.
No additional persistent tests or live provider calls are included here.
