---
id: "02-shared-admission"
title: "Share task and inference capacity with ordered settlement"
status: done
wave: 2
depends_on:
  - "01-child-owner"
plan: "plan.md"
requirements:
  - REQ-AGENTS-INFERENCE-OWNERSHIP-002
acceptance_criteria:
  - AC-AGENTS-INFERENCE-OWNERSHIP-002.1
  - AC-AGENTS-INFERENCE-OWNERSHIP-002.2
  - AC-AGENTS-INFERENCE-OWNERSHIP-002.3
  - AC-AGENTS-INFERENCE-OWNERSHIP-002.4
  - AC-AGENTS-INFERENCE-OWNERSHIP-002.5
system_design:
  - ../../specs/agents/system-design/inference-attempt-ownership.md
  - ../../specs/agents/system-design/session-concurrency-ceiling.md
---

# Task 02: Shared admission and ordered settlement

Join task launches and owned inference attempts in the existing session ceiling
owner. The per-instance owner supplies exact attempt identity and terminal
evidence; the backend persists the exact capacity binding before starting a
child, admits the binding, and settles it in a fixed order.

## Outcome and scope

Finite `all` capacity counts task-launch rows and owned inference bindings in the
same overall and grade limits. Existing ownership keeps its slot across settings
changes; unknown population still refuses new admission. The current inventory is
kept separate from retained historical continuity. One unreadable execution
makes the population unknown for new admission while held bindings remain
counted; it never blanks them.

Teardown forwards the owner's stop observation through the retained workspace
stop path. Only a proven no-start or observed owned-process termination settles an
attempt; cancellation, HTTP errors and transport loss remain unknown. Terminal
observation, provider settlement, binding removal and retained-token ACK keep
their existing order. A recovered run owner leaves the capacity census only after
its terminal state is persisted.

## Out of scope

No new database, queue, poller, counter or background census. Provider-usage
policy, Office/run-session capacity, awaited-stop transports and control UI are
separate slices. The optional capacity policy remains off by default; native
execution, model/profile ordering and agentctl authentication are unchanged.

## Executed verification

Backend build and focused existing checks pass:

```text
go build ./...                                   pass (apps/backend)
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build   pass (utility, process, runtime/agentctl)
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build  pass (utility, process, runtime/agentctl)
go test ./internal/system/sessioncapacity        pass
go test ./internal/agentctl/journal              pass
go test ./internal/agentctl/server/config        pass
go test ./internal/agentctl/server/utility       pass
go test ./internal/agent/runtime/agentctl        pass
go test ./internal/orchestrator -run 'TestAdmit|TestCeiling|TestHandleAgentProcessStart|TestReleaseCeiling|TestConfirmCeiling|TestEvaluateCeiling|TestDeferCeiling'  pass (34.9s)
go test ./internal/agent/runtime/lifecycle -run 'TestManagerExecuteInference|TestStopAgentWithReason|TestStopAgentForce|TestRetainedWorkspaceBlocksDestructiveRuntimeStop|TestStandaloneExecutorStopInstance|TestRunOwner|TestStopTaskExecutionPersistsStoppedRuntimeAndResumeToken|TestStopAllAgents'  pass
go test ./internal/agent/runtime/lifecycle -run '^TestRunOwnerRegistrationPersistsWithoutTaskSession$'  pass
go test ./internal/backendapp -run 'Inference|Capacity'  pass
go test ./internal/agentctl/server/process -run 'TestNewManager|TestManagerJournalClosesWhenTeardownDrainExpires'  pass
go test ./internal/system/sessioncapacity -run '^TestInferenceBindingStoreRoundTripsExactIdentity$'  pass
go test ./internal/orchestrator -run '^TestInferenceAdmissionLimitClassificationAndExactOwner$'  pass
go test ./internal/backendapp -run '^TestNativeInferenceRoot|^TestInferenceRefreshReadFailureKeepsHeldBindingAndFailsClosed$'  pass
go test ./internal/agentctl/server/utility -run '^TestAttempt|^TestRetainedHandoff'  pass
```

Persistent regression cases live in existing test files:
`sessioncapacity/store_test.go` (exact binding identity), `orchestrator/ceiling_release_test.go`
(admission limit classification and exact owner), `backendapp/message_queue_settings_test.go`
(root settlement plus read-failure isolation), and `agentctl/server/utility/runtime_observation_test.go`
(child attempt prepare/consume/cancel and retained-handoff identity/active/continuity guards).

Disposable overlay proofs outside normal collection also pass:

```text
orchestrator: TestInferenceCapacity* (7 cases, 5.709s)
agentctl utility: TestAttempt* (6 cases incl. Windows Job proof, 2.881s)
backendapp: TestNativeInferenceRoot* and TestNativeInferenceStop* (2.283s)
hostutility: TestInferenceConsumerHost* (0.157s)
lifecycle: TestInferenceConsumer*, TestInferenceStop* (0.132s)
backendapp: TestOneOffReadFailureFailsClosedButKeepsBindings, TestOneOffInventoryFailureIsNotMaskedByGatedMode and TestOneOffAutomaticPolicyUsesNativePathWithoutBindingStore and TestOneOffPolicyFlipDuringWaitUsesNativePath
```

The full api and lifecycle suites still hit pre-existing Windows environment
failures (PTY unsupported, Unix permission/mode assertions, missing `sh`, and
Cursor MCP/git timeout cases). Those cases are outside this slice; focused
affected checks above pass. `pr-docs.cjs` `validateCoverage` locally returned
`status: covered` with `errors: []`.
