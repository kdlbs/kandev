---
created: 2026-10-02
status: complete
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-001
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-004
system_design:
  - ../../specs/agents/system-design/managed-npm-runtime-recovery.md
legacy_specs: []
---

# Fix plan: Managed npm startup resilience

## Overview

Recover temporary managed npm setup failures automatically within the original launch operation.
This is the primary package for [issue 4152](https://github.com/kdlbs/kandev/issues/4152), following the user's explicit priority change.
The earlier [failed-session inspection package](../superseded-failed-session-recovery/plan.md) is deferred secondary work.
No implementation from that package is required here.

Agents owns this contract because managed runtime selection, process startup, and setup recovery belong to that system.
Implement structured startup evidence first, bounded recovery second, and complete UI/executor proof third.
The work orders execute sequentially. Tasks 01, 02, and 03 are complete.

## Confirmed gap and evidence

The original npm-race hypothesis remains unproven. The implementation gap is independently established:
`managedRuntimeNpmStartupFailure` accepts only exact-package ETARGET or release-age policy evidence.
`prepareManagedRuntimeStartupRetry` therefore rejects npm ECONNRESET and silent pre-handshake exits.
This matches the existing narrow contract but does not meet the user's expanded recovery requirement.

A temporary `TestIssue4152RetryDiagnostic/transient_npm_network` used the real lifecycle retry function with the existing mock agentctl server.
The server supplied `npm error code ECONNRESET`. The assertion expecting a retry failed with `attempted=false` and no stop/start actions.
Command: `(cd apps/backend && go test ./internal/agent/runtime/lifecycle -run '^TestIssue4152RetryDiagnostic$' -count=1)`.
The temporary diagnostic was removed. This proves missing recovery coverage, not the incident's first process-exit cause.
The attempted empty-stderr control was discarded because the existing fixture substitutes ETARGET whenever its list is empty.
Permanent tests must allow genuinely empty stderr explicitly.

Fourteen isolated adapter 0.81.2 starts succeeded with Node 24.16.0 and npm 11.13.0.
They included cold/warm concurrent starts sharing prefix/cache and initialization delayed one second.
A separate Go reproduction produced both reported pipe errors after a normal child exit.
These findings are retained in the secondary package's investigation section.
They support bounded early-exit recovery without claiming npm caused the original exit.

Two source risks must be addressed in this package:

- `stopAndRepairManagedRuntime` currently logs a stop failure and continues toward replacement.
- Automatic task and host-probe recovery delete the shared package execution tree. Another task can still depend on that tree.

## Scope

### In scope

- One retry for strict ETARGET, recognized transient npm failures, and confirmed unexpected ordinary process exit before initialization.
- Typed phase and process-generation evidence; permanent-error precedence; cancellation and cleanup guarantees.
- Two-second backoff plus zero-to-one-second jitter, within a 90-second recovery budget and the caller's earlier deadline.
- Same session/runtime identity, generation fencing, and single prompt delivery.
- Non-destructive automatic task/probe retries, existing inline progress/failure UI, seven-language copy, and public guidance.
- Local PC, local Docker, and remote SSH task-launch coverage.

### Out of scope

- Proving a shared-directory npm race, guaranteeing success during persistent outages, or serializing all agent launches.
- More than one replacement, version rollback, registry changes, dependency substitution, or global cache deletion.
- New post-initialize retries, native/Codex app-server or passthrough expansion, and unsupported executors.
- Broad host capability-probe retry eligibility; probes keep strict ETARGET matching.
- Automatic changes to explicit Settings maintenance, session selection, or permitted multi-session execution.

## Technical approach

The [design](../../specs/agents/system-design/managed-npm-runtime-recovery.md#bounded-setup-retry)
and [decision](../../decisions/2026-10-02-bounded-managed-npm-startup-retry.md) own the full contracts.

1. Agentctl records generation-scoped process exit and canonical npm diagnostics. Start and initialize messages carry optional generation evidence.
2. The backend preserves a typed initialize-phase failure. The new transient-npm and ordinary early-exit classifications require a trusted managed ACP command plus exact-generation evidence. Existing exact-package `ETARGET` handling remains compatible with peers that do not send that evidence.
3. One startup owner consumes `beginStartupRecovery`, confirms cleanup, delays, and starts the same command.
4. npm-classified recovery uses online-preferred metadata. Unexplained early exit preserves the original preference. Neither deletes shared cache files.
5. Existing generation fences suppress obsolete exits. Final failure passes through the existing recovery pipeline once.

Existing peers without new evidence retain strict ETARGET handling, but no heuristic early-exit fallback.
Match both offline- and online-preferred trusted command shapes for the new classification; do not reject a valid managed command solely because it already prefers online.
Never derive a command, package, path, or registry from stderr.

| Provider/transport/executor | Behavior | Verification |
| --- | --- | --- |
| Claude ACP and other registered managed npm ACP, local PC | Bounded setup retry | Real subprocess + lifecycle + desktop/phone E2E |
| Managed npm ACP, local Docker | Same executor-local stop/restart | Containers E2E |
| Managed npm ACP, remote SSH | Same remote instance/environment | Containers SSH E2E |
| Host capability probe, strict ETARGET | Existing one retry, no tree deletion | Hostutility tests and host probe E2E |
| Native, passthrough, Codex app-server | Existing behavior | Negative classification tests |
| Sprites, remote Docker, Kubernetes | No new automatic recovery | Runtime allowlist tests |
| Legacy agentctl without evidence | Strict ETARGET only | Wire compatibility tests |

## UI-01: Automatic startup retry

Entry: open the launching task while recovery is pending. Use existing boot progress.

```text
Desktop and phone:
[ Starting agent                               ]
[ Retrying agent startup (attempt 2 of 2)       ]
[ spinner; existing task/session navigation    ]

Success:
[ Normal conversation in the same session      ]

Exhausted, desktop:
[ Agent could not start                        ]
[ Startup failed again after one retry.        ]
[ Retry runtime ]  [ Technical details >       ]

Exhausted, phone:
[ Agent could not start                        ]
[ Startup failed again after one retry.        ]
[ Retry runtime                               ]
[ Technical details >                         ]
```

Use the existing recovery card and phone stacked actions; no new modal or scroll owner.
Titles distinguish npm setup failure from unexplained process exit. Wording above illustrates the latter.
Cleanup refusal says the previous process could not be stopped, not that a second attempt failed.
Progress is passive. The explicit retry action appears only after final failure.
All product copy is localized. Phone hit targets remain at least 44px; desktop retains its current density.
Criteria: AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.5 and 004.8.

## Tests and acceptance mapping

| Criteria | Required evidence |
| --- | --- |
| 004.1-.3 | Typed initialize evidence, strict npm-code matrix, truly empty stderr, permanent conditions, post-init exclusions |
| 004.4 | Two attempts maximum; cancel during delay/cleanup; cleanup error prevents spawn; parent deadline wins |
| 004.5 | Original IDs and state through retry; accepted prompt once; one final recovery card |
| 004.6; 001.6; 002.2 | Shared-tree sentinel survives task and probe retry; sibling remains live |
| 004.7 | Three concurrent launches with healthy, recovered, and exhausted outcomes; delayed first-attempt events |
| 004.8 | Safe evidence and accurate copy for npm, unexplained exit, and cleanup refusal |

Deterministic fault-injection fixtures are the CI oracle. Real npm smoke tests already passed but cannot reproduce an intermittent incident on demand.
Do not use live registry access or provider credentials in the regression suite.

## E2E tests

Extend the executor-local npx fixture with per-launch failure modes: transient npm, silent exit, permanent refusal, repeated failure, and normal success.
Use independent scenario IDs and atomic attempt counters, even when prefix/cache are shared.
The fixture must refresh metadata without requiring Kandev to delete a stale marker.
Keep a sentinel representing files read by a healthy sibling and assert it survives recovery.

Add desktop and phone `managed-runtime-startup-retry.spec.ts` variants for progress, success, exhaustion, and cancellation.
Extend existing Docker and SSH managed-runtime recovery specs to exercise the actual restart on their execution hosts.
Include host capability-probe ETARGET tests after removing automatic deletion.
API-seeded cards alone do not prove automatic startup recovery.

## Work orders

- [x] [Task 01: Preserve trustworthy startup evidence](task-01-startup-evidence.md)
- [x] [Task 02: Recover setup without disrupting siblings](task-02-bounded-recovery.md)
- [x] [Task 03: Present and prove startup recovery](task-03-recovery-experience.md)

Execute sequentially in this session unless the user later authorizes delegation.
The completed executor-local and host-probe packages remain historical records; do not rewrite their past test results.
The new ADR qualifies their deletion policy. This package owns the new contract and regression matrix.

## Verification results

Design validation passed:

- `python3 scripts/list-docs.py validate`: 342 decisions and 1295 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- PR documentation coverage preflight: all three work orders covered, zero errors, using a projected production change to exercise the gate.
- `git diff --check`: passed.

Task 03 implementation and verification passed. This includes backend and web builds, focused lifecycle and component tests, typecheck and i18n validation, desktop and mobile browser tests, real Docker and SSH executor tests, public documentation checks, specification validation, and whitespace validation. See [Task 03 results](task-03-recovery-experience.md#results) for the executed commands and counts.
The temporary ECONNRESET diagnostic produced the expected behavioral failure and was removed.
At the initial package handoff on 2026-10-02, issue 4152 remained assigned to carlosflorencio and no implementation commit or PR had been created. PR #4172 was opened later from this package.

Code-review remediation completed on 2026-10-02. The Task 02 results record the three repaired findings and validation. All affected backend package tests, review-specific race tests, the scoped backend linter, the full backend build, specification validation, and whitespace validation passed. The original incident cause remains unknown.

## Risks and assumptions

- One retry is the chosen bound, matching the existing replacement-generation model. The user requested automatic recovery but did not prescribe a retry count.
- Backoff and the recovery deadline are implementation defaults, not new operator settings.
- Empty stderr is not proof of npm failure. Only confirmed early process exit grants the generic pre-handshake retry.
- Unknown exit ownership, permanent conditions, and signal termination fail closed. They can require operator action.
- Non-destructive retry cannot guarantee repair of a corrupted execution tree; explicit maintenance remains available.
- No new code promises that every burst succeeds. Tests prove recovery and isolation for the supported fault classes.
- The original issue's exact process-exit cause remains unknown. This package addresses the failure to recover from eligible setup faults.
