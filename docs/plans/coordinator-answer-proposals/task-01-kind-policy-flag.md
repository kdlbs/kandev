---
id: "01-kind-policy-flag"
title: "Answer kind, policy class, flag, tool and delivery"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-ANSWER-001
  - REQ-COORDINATOR-ANSWER-002
  - REQ-COORDINATOR-ANSWER-004
acceptance_criteria:
  - AC-COORDINATOR-ANSWER-001.1
  - AC-COORDINATOR-ANSWER-001.2
  - AC-COORDINATOR-ANSWER-001.3
  - AC-COORDINATOR-ANSWER-001.4
  - AC-COORDINATOR-ANSWER-001.5
  - AC-COORDINATOR-ANSWER-001.6
  - AC-COORDINATOR-ANSWER-001.7
  - AC-COORDINATOR-ANSWER-001.8
  - AC-COORDINATOR-ANSWER-001.9
  - AC-COORDINATOR-ANSWER-001.10
  - AC-COORDINATOR-ANSWER-002.1
  - AC-COORDINATOR-ANSWER-002.2
  - AC-COORDINATOR-ANSWER-002.3
  - AC-COORDINATOR-ANSWER-002.4
  - AC-COORDINATOR-ANSWER-002.5
  - AC-COORDINATOR-ANSWER-002.6
  - AC-COORDINATOR-ANSWER-002.7
  - AC-COORDINATOR-ANSWER-004.1
  - AC-COORDINATOR-ANSWER-004.2
  - AC-COORDINATOR-ANSWER-004.3
  - AC-COORDINATOR-ANSWER-004.4
  - AC-COORDINATOR-ANSWER-004.5
system_design:
  - ../../specs/coordinator/system-design/answer-proposals.md
---

# Task 01: Answer kind, policy class, flag, tool and delivery

## Summary

Register `answerKind`, add `ActionAnswer` (non-raisable) to the policy, add the
`coordinatorAnswer` flag through `/runtime-feature-flags`, expose
`propose_answer_kandev`, and deliver approvals through the clarification
resolver and the permission response operation.

## Scope

`kinds.go` (registry order), new `kind_answer.go` (with `OpenMatcher` and
`SupersedeOnPropose` hooks and `normalizeDraft`), `propose_kinds.go` (call the
hooks), `relay.go` (`readTargets` refactor and the `AnswerTargets` seam),
`store_proposals.go` (`RefuseOpenTx`, `RefuseClaimedTx`, the fenced claim), `answer_stale.go` (`settleRefusedTx`, `*execRefusal`; the supersede-on-propose and approve-time refusals need them), `approve.go` and `approve_kinds.go` (`kindAction`, `recheckPolicy` skips the class for `answer` and
checks only the flag), `decision_phase2.go` (`knownProposalKind`), `policy.go`
(`ParsePolicy` and `PhaseOnePolicy` keep absence; `deniedActions()` keeps iterating `AllActions` (seven, `answer: denied`) and both unreadable-policy paths deny all seven; the settings write merges a stored `answer` entry into the request policy when the request omits it; `parsePolicyMember` gains `answerAllowed`, guided setup passes false), `settings.go` (optional `answer`),
`reads_phase2.go` (`policyFor` resolves the default and forces denied with the
flag off), `automatic_lower.go` (test that absence survives), `toolprofile.go`
(`proposeTool`, `actionTools`, `proposeOrder`), `reply.go` (`replyTexts`),
`mcpcontract/contract.go`, `internal/mcp/server/coordinator_tools.go` plus its
raw-argument handler wrapper, `internal/mcp/handlers/coordinator_authorization.go`,
`coordinator_propose_kinds.go` (the typed-refusal and read-error responses) and `handlers.go`, `internal/mcp/profile/coordinator_policy.go`,
`internal/clarification` (`ValidateOutcome`, `BundleQuestions` exports),
`runtimeflags/registry.go`, `profiles.yaml`, the frontend flag mirror, the
creation-default policy, tests per AC including the two-approver race and the
in-place-vs-approve race, `answer_pending`, `approval_in_progress`, the
failure-mapping table (one test per row), denied-class pending approval, and
a refused row leaving the unique index, a claim that loses to a row since settled `outcome_unknown`, and cited standing orders checked and marked applied.

## Exclusions

Activity summary counts, the 409 refusal body and the approved-detail builder (task 02); UI (task 03).

## Acceptance

- Propose, dedupe, supersede-on-propose, edit and approve behave as the ACs,
  with exactly one delivery under both races.
- `automatic` for `answer` is refused; absent entry reads per AC-004.2 and
  stays absent after a settings save that omits it and after a lowering.
- Flag off: tool absent, propose and approve refused.

## Verification

```bash
cd apps/backend && go test ./internal/coordinator/... ./internal/mcp/... ./internal/runtimeflags/... -count=1
```
