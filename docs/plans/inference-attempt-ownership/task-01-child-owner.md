---
id: 01-child-owner
title: Observe and settle actual inference child attempts
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-AGENTS-INFERENCE-OWNERSHIP-001
acceptance_criteria:
  - AC-AGENTS-INFERENCE-OWNERSHIP-001.1
  - AC-AGENTS-INFERENCE-OWNERSHIP-001.2
  - AC-AGENTS-INFERENCE-OWNERSHIP-001.3
  - AC-AGENTS-INFERENCE-OWNERSHIP-001.4
system_design:
  - ../../specs/agents/system-design/inference-attempt-ownership.md
legacy_specs: []
---

# Task 01: Inference child owner

Connect the per-instance handler, actual ACP/Codex spawn and cleanup, existing
owner journal metadata, authenticated observation routes and backend client.
Keep unknown outcomes explicit after cancellation, transport loss and restart.
The implementation worker owns source/docs only; integration owns Git and
publication. No model, operating database, profile policy or shared cap is changed.

Overall/grade reservation and utility/review call-site integration remain
original subsequent work. This task alone does not enforce shared capacity.

## Implemented owner boundary

The per-instance handler joins actual ACP/Codex Start, dispatch and common
cleanup to an owner registry and existing journal metadata. Native fresh UUID
issuance identifies intentionally volatile host-utility instances; configured
storage failures and explicit-ID recovery remain unknown. BeginOwnedOperation
keeps cleanup and final metadata writes inside the manager's existing teardown
drain. Windows settlement requires Job TerminateAndWait. Unix recovery observes
the saved process group without signaling a potentially reused group; missing
group can settle an individual attempt, but a restarted owner never attests a
complete historical census. A backend reconnect to the same live owner retains
its actual current inventory. These distinctions remain visible in the DTO.

Independent review reproduced a retired-token replay: after terminal metadata
was pruned, the same explicit token started a second actual child. The accepted
correction uses server prepare, one-time consume and prepared-only cancellation
in the same registry. Missing/pruned tokens reject execution. A client-generated
preparation ID enables cleanup after a lost response and fences late prepare;
running children cannot be settled by that operation. Unresolved old-owner entries
are not retention candidates. No unbounded tombstones, new database or clock
identity was introduced.

## Executed verification

Commands run from apps/backend. OVERLAY and the synthetic protocol child live in
the assigned transient probe location outside normal test collection. The child
implements local ACP/Codex frames; no provider or model is invoked. Production
HTTP auth, instance guard, runtime client, process manager, utility executors,
cleanup and retained bbolt owner metadata are used.

```text
go test -trimpath -p 2 -race -overlay OVERLAY ./internal/agentctl/server/api ./internal/agentctl/server/utility ./internal/agentctl/server/instance -run '^TestInferenceOwner' -count=1 -timeout 90s -v
PASS: api 13.890s; utility 6.082s; instance 1.602s.
```

The cases are TestInferenceOwnerActualHTTPChild (ACP and Codex),
TestInferenceOwnerRestartAndUnknownActualChild, TestInferenceOwnerBoundedRetention,
TestInferenceOwnerRetiredTokenRejectsReplay and TestInferenceOwnerNativeFreshInstance.
They verify live/readback after a new backend client, actual termination,
cancel-then-late settlement, duplicate rejection, native auth, no content/PID/path
exposure, no attempt for probes, not-started results, old-peer observation errors,
actual teardown saving settlement before journal close, unknown missing-Job and
restart outcomes, bounded retention, retired-token rejection with exactly one
actual child start, prepared-only cancellation, running-token cancellation denial,
lost-prepare-response cleanup, late preparation fencing and native fresh vs
explicit-ID inventory. Raw before-replay output remains with the integration
owner; the corrected probe and raw results are retained for readback.

```text
go test -trimpath -p 2 ./internal/agent/runtime/agentctl ./internal/agentctl/server/utility ./internal/agentctl/server/api ./internal/agentctl/server/instance ./internal/agentctl/server/config -run '^Test(InferencePrompt|LongRunningJSON|WindowsACPCommand|ApplyInstance|InstanceConfig|NewInstanceConfig)' -count=1 -timeout 90s
PASS: runtime client 0.176s; utility 1.172s; config 0.026s.
API and instance compiled with no matching tests in this command; their actual
owner cases are the disposable race invocation above.

go test -trimpath -p 2 ./internal/agentctl/server/utility -run '^Test(WindowsACPCommand|ExecuteACPSession|ResolveCodexAppServerCommand|CodexUtilityMCPConfig|ProbeACPSession|BuildACPCommand|SanitizeEnvForAgent)' -count=1 -timeout 90s -v
PASS: utility 12.627s. Existing model, gateway, command/environment and Job checks.
```

Linux amd64 and Darwin arm64 utility test binaries cross-compiled with
CGO_ENABLED=0, go test -trimpath -p 2 -c. No Unix child was executed on this Windows
host. Unix restart behavior is supported by the existing Setpgid/Pdeathsig source
and explicit group-presence observation; it is not claimed as a Unix runtime
result. Existing journal focused tests passed (1.854s), API checks passed (1.206s)
and runtime client checks passed (0.112s) in the initial owner increment. That
initial broad-name command compiled utility but selected no utility tests; the
actual relevant checks above supplied its verification.

The first four-package lint reported only an unchecked response Body.Close in
the new runtime client. Applying the existing deferred-close pattern and checking
that client delta produced exit 0 / 0 issues. Final prepare/consume and fresh-owner
validation completed as follows, using process-local GOMEMLIMIT=3GiB,
GOMAXPROCS=2 and GOFLAGS=-p=2 after the shared lint slot was returned:

```text
golangci-lint run ./internal/agentctl/server/utility ./internal/agentctl/server/api ./internal/agentctl/journal ./internal/agent/runtime/agentctl ./internal/agentctl/server/config ./internal/agentctl/server/instance --new-from-rev=HEAD --timeout=5m --concurrency=2
exit 0; 0 issues.

go test -trimpath -p 2 -race -overlay OVERLAY ./internal/agentctl/server/utility -run '^TestInferenceOwner(BoundedRetention|RetiredTokenRejectsReplay)$' -count=1 -timeout 60s -v
PASS; utility 3.636s after the final unresolved old-owner retention correction.

python scripts/lint-spec-files.py --all
All specification files passed.
python scripts/list-docs.py validate
Validated 383 decisions and 1617 specifications.
```

The existing pr-docs.cjs.validateCoverage accepted all 27 changed paths and four
document bodies with status covered and errors []. Diff whitespace checks passed.
No permanent test function or verification runner was added. The integration
owner has the raw independent replay failure and retained corrected proof sources
and output. No model, operating runtime, database, profile setting, Git ref or
publication was changed by the implementation worker.

## Original remaining integration

C2b must bind and persist each server-issued attempt token to the existing shared
overall/grade reservation, connect utility/manual-review/workflow-review callers,
and release only from definite no-start/termination evidence. It must retain old
owner unknowns across reconnection and use the existing recovery owner for an
agentctl restart or unavailable history, explaining the unknown outcome. Old-peer
fallback is native compatibility, not all-policy authority. No counter, grade
admission, priority substitution, new queue, poller, operating configuration or
permanent tests were added here. Integration owns normal commit/publication and
transient cleanup after readback.
