---
id: "01-classify-startup-launch-failures"
title: "Classify only agent startup launch failures as provider failures"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-LAUNCH-FAILURE-001
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-LAUNCH-FAILURE-001.1
  - AC-AGENTS-DYNAMIC-LAUNCH-FAILURE-001.2
  - AC-AGENTS-DYNAMIC-LAUNCH-FAILURE-001.3
  - AC-AGENTS-DYNAMIC-LAUNCH-FAILURE-001.4
  - AC-AGENTS-DYNAMIC-LAUNCH-FAILURE-001.5
system_design:
  - ../../specs/agents/system-design/dynamic-launch-failure-classification.md
---

# Task 01: Classify only agent startup launch failures as provider failures

## Summary

Move the launch error classification out of `dynamicTaskDownstream.Launch`
into `classifyDynamicLaunchFailure` and require agent startup provenance before
a launch error is classified.

## Acceptance

1. Workspace preparation, command validation, runtime detection, and local
   agentctl errors (including `connection refused`, `i/o timeout`, and
   `service temporarily unavailable` texts) return unclassified.
2. An `AgentStartupFailure` with a provider network error returns a classified
   `network_unavailable` failure.
3. An already classified error keeps its code.
4. A current asynchronous dynamic startup with an explicitly unsent prompt
   and a typed requested-model rejection applies the existing candidate policy.
   Fixed profiles, resume failures, stale attempts, and unknown delivery keep
   their existing recovery behavior.

## Asynchronous model rejection implementation

- `apps/backend/internal/orchestrator/dynamic_startup_model.go` accepts only the
  typed rejection and current no-output/no-effect startup evidence.
- `apps/backend/internal/orchestrator/event_handlers_agent.go` associates that
  evidence with the current execution, generation, session and profile.
- `apps/backend/internal/orchestrator/dynamic_launch.go` preserves the proven
  pre-prompt evidence for the existing dynamic candidate policy.
- `apps/backend/internal/orchestrator/dynamic_evidence.go` binds the admitted
  execution before process start and merges the matching observed effects.
- `apps/backend/internal/agent/runtime/dynamic/types.go` carries explicit typed
  rejection evidence rather than treating generic model errors as equivalent.

## Verification

- `go test ./internal/orchestrator/ -run 'TestDynamicLaunch'`
- `make -C apps/backend build`
- On this change, three temporary startup-model boundary and ownership checks
  passed with a Go overlay, as did the existing startup admission and dynamic
  successor launch checks. Specification lint, documentation catalog validation
  and normal commit hooks passed. No live-provider or full-backend run is claimed.
- A follow-up temporary overlay reproduced and rejected unsafe fallthrough when
  startup evidence has output, effects, or unknown status. It also checked the
  same-execution observation merge, absent record, and ambiguous identities.
- A second targeted overlay reproduced early admission before dynamic evidence
  binding, generic model classification without typed rejection, and retired
  state without a record. It verifies exact-attempt pre-start binding, caller
  callback preservation, stale-callback fencing, and the typed/retired guards.

## Permanent regression coverage

`dynamic_startup_model_test.go` covers the typed rejection, missing or false
unsent-prompt evidence, resume failures, fixed profiles, and stale route ownership.
The handler tests use persisted routes to verify both `try_next` and `stop`.

`dynamic_startup_evidence_test.go` covers unsafe evidence, recorded output and
effects, absent records, retired attempts, and stale admission callbacks.
The admission test uses `dynamicTaskDownstream.Launch` and the executor callback
to verify execution binding before process start and preservation of the caller callback.

Focused verification from `apps/backend` passed:

```bash
GOMAXPROCS=2 GOMEMLIMIT=3GiB go test -trimpath -race -p 2 \
  ./internal/orchestrator \
  -run 'TestDynamicStartup|TestDynamicLaunch|TestDynamicAttemptEvidence|TestDynamicPreResult|TestWorkflowAsyncStartFailure_DynamicFallbackOwnership' \
  -count=1
```

The documentation catalog check passed with `python3 scripts/list-docs.py validate`.
Scoped Go lint passed with zero issues for the same package.
The default wrapper reported zero issues but timed out. A serialized retry with
a ten-minute limit passed:

```bash
task_lint_base=$(scripts/resolve-go-lint-base --comparison-base)
cd apps/backend
export GOMEMLIMIT=3GiB GOMAXPROCS=2
export GOFLAGS="${GOFLAGS:+$GOFLAGS }-p=2"
golangci-lint run ./internal/orchestrator \
  --new-from-rev="$task_lint_base" --timeout=10m --concurrency=2 \
  --allow-serial-runners
```

Public documentation needs no update because this follow-up adds coverage without
changing production behavior.
