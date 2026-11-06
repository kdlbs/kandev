---
id: 15-dynamic-candidate-timeout
title: Share native review routing and stop unconfirmed request deadlines
status: in_progress
wave: 11
depends_on: ["04-run-orchestration"]
plan: plan.md
requirements:
  - REQ-AGENTS-NATIVE-REVIEW-TIMEOUT-001
acceptance_criteria:
  - AC-AGENTS-NATIVE-REVIEW-TIMEOUT-001.1
  - AC-AGENTS-NATIVE-REVIEW-TIMEOUT-001.2
  - AC-AGENTS-NATIVE-REVIEW-TIMEOUT-001.3
  - AC-AGENTS-NATIVE-REVIEW-TIMEOUT-001.4
system_design:
  - ../../specs/agents/system-design/native-review-timeout.md
legacy_specs: []
---

# Shared native review routing

Use the existing profile resolver, policy engine and utility execution loop for
native review. Keep one isolated route for the run and retain the candidate and
generation across batches. Preserve the incoming abandoned-run recovery work
and its independent task claim and cancellation settlement boundaries.

## Delivered scope and remaining acceptance

Classified provider failures reach saved policy before same-candidate host
fallback, including bounded pending
retry waits. Partial output, parent cancellation and typed capacity refusals stop
before another candidate or a host bypass. Every dispatched failure reaches its
policy, including the eighth attempt in each prompt batch. That batch permits no
ninth dispatch or extra wait.
Server-read task, session and review-run relationships reach inference alongside
logical and concrete profile identity. The existing detached ten-second terminal
failure writer preserves both execution and storage errors.

AC-AGENTS-NATIVE-REVIEW-TIMEOUT-001.2 remains incomplete: a transport deadline
with a live parent cannot establish exact child termination. Such deadlines stay
unclassified and never authorize automatic fallback. The child owner must supply
that proof in later work; this change adds no admission subsystem, provider
claims, retry ledger, polling owner or timeout configuration.

Bound utility/default profiles preserve blank-model behavior. Explicit reviewer
profiles retain their existing validation. The unused InferenceConsumer
interface is removed; the actively used identity context and typed capacity
error boundaries remain.

## Permanent regressions and verification

- `internal/utility/service/profile_execution_test.go`: A-to-B success and
  context attribution; partial response plus error, unknown wrapped deadlines,
  parent cancellation and capacity refusal; eight dispatches/eight reports with
  no ninth dispatch or wait; pending timer, nil deadline and cancellation.
- `internal/backendapp/review_routing_test.go`: actual profile resolver and
  dynamic engine over isolated SQLite profile storage, native adapter and runner,
  route retention across batches, stale generation fencing, pending-route resume
  identity, server-verified task/session/run relationships and stop conditions.
- `internal/utility/handlers/routing_identity_test.go`: registered HTTP session
  and host paths retain the server-created call identity through fallback and
  persist the selected concrete profile. Browser-supplied identities are ignored.
- `internal/review/routing_contract_test.go`: bound blank-model versus explicit
  validation, real ReviewService/SQLite terminal failure, detached write context,
  and joined execution/storage causes.

Run the affected packages under race in hosted CI:

```sh
go test -trimpath -tags fts5 -race ./internal/utility/service ./internal/utility/handlers ./internal/backendapp ./internal/review -count=1 -v
```

Hosted focused proofs must distinguish assertion RED from setup/compiler errors,
retain unchanged positive controls, and verify corrected GREEN. Full final-head
backend/frontend/E2E families and named platform contracts remain delivery gates.
No contributor code, dependency install, compiler or test runs on the review host.

## Results

The first hosted proof failed before its mutation phase: pointer-receiver fixture
errors prevented review-package compilation; provider catalogue mismatches stopped
synthetic routing and pending-policy cases; an unrelated whole-package sweep hit
its 120-second test budget. That attempt earns no mutation RED or GREEN credit.
Corrected focused proofs select all new routing/handler/runner regressions and
unchanged controls without increasing budgets. Full unmodified CI owns the complete
packages and platform matrix. Exact source-bound results, attempts, raw logs,
artifacts and review dispositions live in the external PR delivery evidence and
PR validation record. Historical contributor disposable-probe timings are not
acceptance evidence. No live-provider or installed-runtime timeout recovery
acceptance is claimed.
