---
created: 2026-10-07
status: in_progress
requirements:
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-002
system_design:
  - ../../specs/platform/system-design/agent-settings-parity.md
  - ../../specs/platform/system-design/profile-enabled-omission.md
legacy_specs: []
---

# Implementation plan: Preserve profile availability during metadata edits

## Overview

Preserve a committed enabled toggle when an ordinary or dynamic profile save
omits enabled, and project the flag committed by that save into its response and
existing notifications. Deliver one sequential work order across the shared
controller/storage boundary, preserving legacy explicit full-row writes.

Platform settings parity owns omission and equivalent saved results. The
[Agents selection specification](../../specs/agents/requirements/profile-disable.md)
explains the user impact and remains unchanged. The historical
[profile mutation work order](../agent-settings-parity/task-02-profile-mutations.md)
is done; its sequential omission tests do not prove concurrent preservation.
This bounded corrective package does not reopen the whole settings migration or
change that historical delivery result.

## Scope

### In scope

- Enabled request presence through ordinary and production atomic dynamic saves.
- Statement-captured enabled results, current timestamp/user-modified semantics,
  dependency confirmation and dynamic rollback compatibility.
- Real database/controller regressions, faithful REST and guarded MCP returns
  and existing events, physical PostgreSQL behavior, native Windows execution.
- Minimal repository/fake interface adaptation and required persistence gates.

### Out of scope

- Other omitted fields, Office/general writers, runner admission, frontend
  selectors, session launch/resume behavior, new schemas/APIs/revisions/flags.
- UI layout, copy, touch, navigation, browser runtime and new Playwright work.
- Speculative settings adoption, global advisory locking or broad writer audits.

## Evidence and source reconciliation

Own source base is `e4f11385ec772d421ab55b90e76c750a92233c02`; the initial worktree
was clean and matched ROOT's proof base. ROOT accepted a read-only causal SQLite
proof using real `Controller.UpdateProfile` instances and the production store.
Native handle `72565/5fb638` was actually joined by `be8498`, exit 1, Go 0.056s.
Rename true-to-false and model false-to-true interleavings failed stored/returned
assertions; uncontested-disabled rename and explicit-mixed enabled controls
passed. No HTTP, physical PostgreSQL, or dynamic execution is claimed.

The protected proof is `/tmp/kandev-root-profile-enabled-omission-candidate_test.go`,
mode 0400, SHA256
`73fd54ccf993e6380db29e357373eeb86275eaf800de52a01784ed63fc8f9922`.
Receipts live under `/tmp/kandev-root-profile-enabled-discovery-20261007/`
(`qualification.json`, `proof-receipt.json`, `native.json`, `overlay.json`,
`proof.log`). Do not replay, import, mutate or delete these before actual merge
and ROOT archive. Permanent tests are independently owned by Task 01.

## Technical approach

Use the [focused design supplement](../../specs/platform/system-design/profile-enabled-omission.md)
for the source inventory, exact internal method contracts, SQL and projection.
Change `controller/profile_crud.go`, `store/store.go` and `store/sqlite.go` with
one shared parameterized enabled assignment and returned column. Keep the
legacy methods as explicit full enabled wrappers and the existing enabled-only
method narrow. Pass request intent into `updateDynamicProfileAtomically` and
the production atomic transaction; version/route failures roll back the base.

| Interface/provider shape | Intended result | Planned evidence and fallback |
| --- | --- | --- |
| REST, real SQLite controller/store | Omitted/null enabled preserves toggle; response and existing broadcast use this write's value | Actual handler + DB interleaving and event spy; native Windows runs the same applicable cases |
| Registered compatibility MCP + guarded dispatcher | Supported name/model omission and existing schema/authority remain; response/event use saved DTO | Real schema/dispatcher/controller/DB, transport-only mocks |
| Compact MCP + actual backendapp operations | Correct sanitized saved profile and timestamp | Real adapter/controller/DB; no compact event claim where source has no publisher |
| SQLite atomic dynamic extension | Base and routes remain one versioned transaction | Real controller route-save and stale-version/parent failure matrix |
| PostgreSQL production repository | Same column assignment and statement result under actual row wait | Physical independent connections; commit, held-toggle rollback, dynamic conflict; DSN absence is a reported skip, never delivery evidence |
| Non-atomic lightweight dynamic adapter | Existing route-update fallback ordering with intent-aware base save | Bounded adapter control; no expanded atomicity promise |
| Legacy full-row calls | Explicit enabled replacement remains | Direct real-store controls, working-owner and model-adoption regressions |

No schema migration or new persistence descriptor is needed. Required SQL guard
and store conformance still run because SQL persistence changes.

## Tests

Use the `TestProfileEnabledIntent` prefix for new tests, with table subcases to
avoid duplicate harnesses. Task 01 names the six permanent test files and complete
matrix. AC-PLATFORM-AGENT-SETTINGS-PARITY-002.2/002.10 map to causal caller/store
interleavings and own-write projection; 002.4 to rejected/error/rollback cases;
002.5 to existing REST/MCP publishers; 002.6 to returned saved state. Existing
exact-model/provider/MCP/dependency controls remain in force.

## E2E evidence

The user-visible availability result is proved end to end through real profile
callers and persistence, including the existing publication boundaries. This
backend data repair changes no rendered or viewport-dependent interaction;
mobile parity uses the producer/data-only exception. Existing selection flows
remain documented in `apps/web/e2e/tests/settings/agent-profile-disable.spec.ts`.
Do not schedule browser/E2E runs for this package or claim new browser evidence.

## Work orders

- [ ] [Task 01: Preserve enabled intent through profile saves](task-01-preserve-enabled-intent.md)

Task 01 is in progress following ROOT's later explicit implementation release
and acceptance receipt at `/tmp/kandev-root-child73-design-acceptance-20261007.json`.
Exactly one work order, wave 1, sequential. No delegates, extra primary sessions,
tabs, model changes, or task-creation workers are authorized.

## Design checkpoint and later delivery

This turn creates only unstaged, uncommitted docs. No production/permanent tests,
Go checks, install, DB, runtime, container, staging, commit or PR runs during
DESIGN. End the turn after cheap doc gates and save the continuation in the own
platform plan. Implementation requires a LATER explicit ROOT implementation
INTERRUPT in this same primary; no operator approval/model-switch prompt.

After that release, follow Task 01's resource-capped TDD/gates and normal active
commit hooks. Publish a ready PR using the same frozen verified SHA unless an
actual finding requires correction. Return GLOBAL heavy admission to ROOT before
the single 90-minute observer. Timeout, transport uncertainty or unknown mutation
requires a ROOT checkpoint; reconcile unknown state before any repeat.

Resolve canonical repository/head and all five operational automation flags
before publication/observation. Preserve legitimate live bot body/checklist
updates. Require all six actual required contexts and actual Backend, Frontend
and E2E parent workflows to succeed, plus actual native Windows and physical
PostgreSQL new cases with RUN/PASS evidence, not build or skipped evidence.
Require authenticated App 347564 substantive FULL review of every actual file:
source=covered=current head, kind reviewed. An ACK is not a review. Accept a
sufficient automatic review first; at most one necessary manual request after
a proven skip/gap and no running/completed FULL review. Disposition every actual
finding; defer optional polish and avoid optional waits.

MERGE NONE until ROOT gives a separate serial grant near actual terminal
readiness. Then only normal expected-head squash, without admin bypass, rebase
or synthetic test evidence. Independently verify actual merged SHA/tree/blobs
and remote inclusion; join every handle and clean only proven owned resources.
ROOT archives and releases the protected proof. Preserve the managed worktree,
dependencies, shared caches, foreign resources, oversized paused task and the
anonymous unproved volume.

## Verification results

Design checks passed: catalog validation (360 decisions, 1432 specifications),
spec-linter tests (36 passed), full specification lint, tracked diff whitespace,
relative file-reference checks for all three new documents, five acceptance
references resolved in exactly one work order, and `git diff --no-index --check`
against `/dev/null` for each new document. All checks exited 0.
Implementation is in progress after ROOT's later release. Independently authored
ordinary/dynamic real-caller RED and current SQLite, REST, registered guarded
compatibility MCP, compact settings and affected compatibility GREEN passed.
Physical PostgreSQL ran without SKIP and passed all eight observed row waits,
rollback and explicit/full-write controls. Persistence conformance with the owned
DSN and SQL guard passed. Exact owned PostgreSQL cleanup passed; no volumes were
created. Scoped lint passed after three bounded style corrections; publication remains in progress; hosted Windows,
PostgreSQL and PR gates remain pending. Task 01 and the own platform plan retain
commands, original handles, joins, receipts and admission state.

Public-doc audit: `docs/public/agents-and-profiles.md`, root README and screenshot
catalog still describe the same profile operations. No public documentation
change is needed for restored omission behavior; no labels or recovery steps
change. The legacy profile-disable dependency wording is superseded by existing
utility dependency safety and current controller checks, both retained here.

## Risks

- Request omission can be lost at ordinary, atomic dynamic, or lightweight
  adapter boundaries. Required intent forwarding and real caller tests constrain it.
- Returning a later reread can misreport this commit; statement results constrain it.
- SQLite-only success does not prove PostgreSQL row-wait evaluation. Require
  distinct physical connections and observable waiting in actual execution.
- A large generic fake migration or concurrency redesign would exceed this
  repair; limit compatibility edits to required interfaces and current wrappers.
