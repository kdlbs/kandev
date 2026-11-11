---
id: "01-launch-observations"
title: "Observe existing OpenCode background launches"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-007
  - REQ-AGENTS-BACKGROUND-WORK-008
acceptance_criteria:
  - AC-AGENTS-BACKGROUND-WORK-007.1
  - AC-AGENTS-BACKGROUND-WORK-007.2
  - AC-AGENTS-BACKGROUND-WORK-007.4
  - AC-AGENTS-BACKGROUND-WORK-007.5
  - AC-AGENTS-BACKGROUND-WORK-008.5
system_design:
  - ../../specs/agents/system-design/background-work-opencode.md
---

# Task 01: Observe existing OpenCode background launches

## Summary

Map the observed OpenCode launch sequence to shared background inspection.
Deliver an honest observation-only result on OpenCode 2.2.0 before continuous
producer support exists.

## In scope

- Add a pure OpenCode launch recognizer. Correlate background=true input with
  the same call's nonempty shellID and running result.
- Preserve the command, source call, origin turn, and opaque shell identity.
  Keep the launch tool complete while the job remains independently observed.
- Use the existing backgroundlaunch registry and shared workload projection.
  Retain the binding after activeToolCalls removes the launch call.
- Preserve ordinary shell normalization, including real exit codes.
  Make unverified legacy jobs unknown after root prompt settlement.
- Propagate the effective shared feature value through existing launch/configure,
  adapter.Config, and shared.Config boundaries. Add explicit typed fields where
  absent; older peers default off. Gate new attestation/projection behavior.
- Include negative cases for malformed input, conflicting identity, another
  provider, disabled feature, and foreground shells.

## Out of scope

The upstream producer, automatic response delivery, new controls, steering,
workflow placement, and new frontend components.

## Acceptance

1. A fixture with three launches produces three stable shared jobs. Completed
   launch tools do not terminate those jobs or overwrite their origin turns.
2. Legacy settlement produces unknown activity and no fabricated exit code,
   final output, automatic-delivery capability, or verified parked status.
3. Feature-off and unrelated providers preserve their existing wire, messages,
   accounting, and prompt admission.

## Verification

From the Kandev root:

~~~bash
(cd apps/backend && go test -trimpath -race ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/adapter/transport/acp/backgroundlaunch -run 'Test(OpenCodeBackground|OpenCodeLaunch|OpenCodeLegacyBackground|NormalizeExecute|UpdateShellExecInput|StampBackgroundShellWork)' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/task/service ./internal/task/repository/sqlite -run 'Test(OpenCodeBackground|BackgroundWork)' -count=1)
(cd apps/backend && go test -trimpath ./internal/runtimeflags ./internal/common/config ./internal/profiles -count=1)
git diff --check
~~~

Use TDD for the proposed TestOpenCodeBackgroundLaunchIdentity,
TestOpenCodeLaunchCompleteIsNotJobComplete, and
TestOpenCodeLegacyBackgroundUnknown tests. Keep the identity assertions at the
full adapter-to-observation boundary, not only in a parsing helper.

## Files likely touched

- Existing ACP normalize.go, enricher_opencode.go, adapter_tools.go, dialect.go,
  and background_launch_recognizer.go.
- Proposed ACP opencode_background.go and opencode_background_test.go.
- Existing adapter/adapter.go, transport/shared/config.go, agentctl request
  types, process configuration, and lifecycle request construction.
- Existing orchestrator/event_handlers_background_work.go and task/service
  background_work.go, only where observation/legacy-unknown wiring needs them.
- Existing shared feature registry/profile tests; profiles remain unchanged.

## Dependencies

None. This result is explicitly partial support, not full continuity.

## Risks

The launch result can omit input and title. Correlation must use prior call
state. An ID printed in content is not structured ownership evidence.

## Parallelism

sequential

## Inputs

- [Design](../../specs/agents/system-design/background-work-opencode.md#existing-launch-observations).
- [Evidence](protocol-evidence.md).
- Existing normalize_background_test.go, background_launch_extensibility_test.go,
  adapter_client_capabilities_test.go, and task background-work tests.

## Results

Implemented on 2026-10-27.

- Added `acp/opencode_background.go`: a pure launch recognizer that correlates
  the `background: true` shell input with the same call's result metadata
  (`shellID`/`shellId` and `status=running`). It rejects missing, malformed, or
  conflicting identity and never parses the human-readable banner.
- Extended the `backgroundlaunch` seam with an optional result-phase
  `RecognizesConfirmedLaunch` and `ConfirmDetachedLaunch`. The existing
  input-phase recognizers (Claude) are unchanged; OpenCode reports false at
  input time and supplies its shell identity only from the tool result.
- `ShellExecPayload.Background` is now set from OpenCode's `background` input;
  the flag is scoped to `opencode-acp` so another provider's boolean is never
  read as an OpenCode launch.
- Added the negative `BackgroundWorkPayload.Unverified` marker and
  `SetUnverifiedBackgroundWorkIdentity`. Confirmed OpenCode 2.2.0 launches are
  retained and inspectable but unverified, so they never attest parked/working
  activity the runtime cannot confirm. An older peer that omits the marker keeps
  the legacy verified meaning.
- After the foreground prompt settles, unverified OpenCode launches become one
  `background_work_updated` observation with `state=unknown`, the original
  shell identity, command-derived title, no exit code, no output, and no
  control capability. They stop fencing later prompts (capacity continuation,
  prompt-end cancellation) and are never reported as cancelled tools.
- Propagated the effective feature value through
  `AgentctlStartupConfig.AgentBackgroundWorkEnabled` -> agentctl server config
  -> instance config -> `adapter.Config` -> `shared.Config`. Older peers omit
  the field and decode false. Validation and JSON round-trip are additive.
- Task-service observations now return the committed projection, so published
  updates carry the durable revision, retained output, and exit status.
- Provider run identity is carried explicitly (`BackgroundWorkPayload.RunID`);
  it is never derived from the workload ID. Native Codex keeps its process-ID
  run while normalized command updates have no run key, so one execution does
  not become two run rows.
- The unknown settlement observation carries the original launch call
  (`SourceCallID`). Source call, source message, parent, origin turn, and the
  capability declaration are immutable once recorded; later updates that omit
  them preserve the stored values.
- The verification state is a negative marker (`unverified`). A previous
  agentctl wire shape that omits it keeps the existing verified/parked
  semantics, while explicitly unverified OpenCode launches cannot attest live
  activity.
- Every prompt-ending path settles retained unverified launches: success before
  the complete event, error returns through a deferred settlement, and user
  cancellation before the generic cancelled-tool sweep.
- Claude ACP shell, subagent, and Monitor producers publish an explicit
  inspect-only capability declaration instead of relying on a read-time
  fallback for metadata or controls.
- An additive `task_session_background_work.capabilities` column stores the
  adapter's declaration (created for fresh databases and added for existing
  databases by the migration stage, with a schema-replay test).
  HTTP/WS hydration reads it, applies state-based action availability, and
  falls back to inspect-only when no declaration exists. This is the
  documented persistence change the plan allowed for a discovered gap.

Tests added: `opencode_background_test.go` (launch identity, launch-complete vs
job-complete, legacy unknown, verified-work preservation, feature off, unrelated
provider, recognizer negative cases), plus startup-contract and profile
propagation tests.

Evidence run:

~~~bash
(cd apps/backend && go test -trimpath ./internal/agentctl/server/adapter/... ./internal/agentctl/types/streams ./internal/agentctl/server/config ./internal/common/config ./internal/profiles -count=1)
(cd apps/backend && go test -trimpath -race ./internal/task/service ./internal/orchestrator -run 'Test(Background|OpenCode|Parked|SecondRegistered)' -count=1)
~~~

All passed. Full-continuity delivery remains out of scope for 2.2.0 and depends
on Task 02.
