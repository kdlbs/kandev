---
created: 2026-10-30
status: in_progress
requirements:
  - REQ-AGENTS-SANDBOX-003
  - REQ-AGENTS-SANDBOX-005
  - REQ-AGENTS-SANDBOX-006
  - REQ-AGENTS-SANDBOX-009
  - REQ-AGENTS-SANDBOX-010
  - REQ-AGENTS-SANDBOX-011
  - REQ-AGENTS-SANDBOX-012
  - REQ-PLUGINS-SANDBOX-002
system_design:
  - ../../specs/agents/system-design/sandbox-evidence.md
  - ../../specs/agents/system-design/sandbox-operation-approvals.md
  - ../../specs/plugins/system-design/sandbox-provider-v2.md
  - ../../specs/agents/system-design/sandbox-policies.md
legacy_specs: []
---

# Implementation plan: expanded Nono evidence and approvals

## Overview

First migrate the dedicated plugin SDK and evaluate Nono 0.81.0. Deliver separate evidence facts and richer audit reports before extending operator approvals to TCP connections, commands, and HTTP endpoints. Finish with typed learning and actual managed acceptance.

This is an extension of the [initial package](../nono-sandbox-integration/plan.md), not a replay of its completed work orders. Host baseline is `29170265fc88b820c661036dc626fcf8d83ccb41`; companion plugin baseline is `8f68c6a10add119a1187c7428c5fad51aa6ca25a`. Existing acceptance predates SDK extraction. The user reviewed this package and authorized implementation. Results below track delivery. The paired pull requests remain drafts while the remaining work orders and acceptance checks are incomplete.

## Scope and assumption check

The user confirmed both proposed phases on October 30. Agents owns effective permission and evidence meaning; Plugins owns optional wire compatibility. Production Nono code remains in its dedicated repository. Existing executor/agent/workflow/task selection precedence and operator-only authority remain mandatory.

Included: standalone SDK migration; explicit 0.81.0 evaluation; separate runtime facts; network/command/endpoint reports; verification of existing finalized audit/signature artifacts; typed operation ceilings and approval envelopes; exact one-shot approvals; desktop/phone parity; explicit learning; and current-pair acceptance.

Excluded: automatic upgrades or rollout, global CA changes, remote executors, Windows/macOS expansion, signed platform enrollment, remote audit ingestion, private-key provisioning, unrestricted matching, and a fabricated continuous-attestation claim. These exclusions do not remove any of the confirmed reporting/status/approval outcomes.

Work remains sequential in the primary session. Do not change main :9998, retained fixtures, the deprecated issue task, or the former parent worktree. Do not merge either PR. No unanswered product preference blocks this package; unsupported runtime behavior is an explicit implementation gate.

## Source evidence

Inspected upstream 0.80.0 at `8bb041356d48f771a221e58c98d9b1ff442e76fe` and the 0.81.0 tag source on October 29-30. These are source observations, not new Kandev acceptance receipts:

- [Audit types](https://github.com/nolabs-ai/nono/blob/v0.81.0/crates/nono/src/audit.rs) already include network, command, and conditional runtime metadata. Our adapter currently filters filesystem denials only.
- [CONNECT implementation](https://github.com/nolabs-ai/nono/blob/v0.81.0/crates/nono-proxy/src/connect.rs) adds network requests with PID zero. Ordinary grants are not cached across connections.
- [Endpoint implementation](https://github.com/nolabs-ai/nono/blob/v0.81.0/crates/nono-proxy/src/reverse.rs) uses PID zero and the shared `proxy` label. Protected attempt-channel binding is necessary.
- [Network policy compilation](https://github.com/nolabs-ai/nono/blob/v0.81.0/crates/nono-cli/src/network_policy.rs) still strips ports from plain allow-domain entries.
- [Approval protocol](https://github.com/nolabs-ai/nono/blob/v0.81.0/docs/protocols/approval-webhook-v1.md) limits network approvals to the direct CONNECT path, not enterprise upstream forwarding.

Runtime source, archive and executable hashes must be captured from verified artifacts in Task 01. No source observation enables a capability.

## Technical approach

The [evidence design](../../specs/agents/system-design/sandbox-evidence.md) defines orthogonal facts and bounded final-artifact verification. The [operation design](../../specs/agents/system-design/sandbox-operation-approvals.md) defines typed grants, envelopes, request actors, native network preflight, and exact command/endpoint mediation. The [provider design](../../specs/plugins/system-design/sandbox-provider-v2.md) defines optional protocol major 2 and v1 preservation. The [ADR](../../decisions/2026-10-30-sandbox-evidence-and-operation-authority.md) records the durable boundaries.

Use existing `Observation`, `boundConfinementEvidence`, `VerifyLive`, report/approval stores, session access checks, WebSocket ordering, and Execution details components. Extend public SDK/protobuf and generated types together. Version persisted payloads and SQLite/PostgreSQL migrations without changing captured hashes. Both host and plugin artifacts must identify their protocol and exact runtime/package identities.

## Compatibility matrix

| Provider/path | Intended result | Evidence gate / unsupported fallback |
| --- | --- | --- |
| No provider, generic wrapper | Existing core wrapping | Regression control; never require plugin installation |
| Major 1 provider and captured policy | Existing behavior and history | Old/new host contract tests; no reinterpretation |
| Major 2 deterministic provider | All generic typed flows | Validator/UI tests only; no real confinement claim |
| Nono 0.81.0, Linux amd64 Local/Worktree | Extended evidence and exact operations | Real packaged checks per class; unsupported policy blocks launch |
| Nono v2 native approval proxy | Exact TCP evaluation per connection | No upstream relay; callback/MCP isolation and port tests required |
| Nono v1 static upstream relay | Earlier exact-destination mechanism | Preserve pinned behavior; no implicit mode switch |
| Other OS/executor/runtime | No new support claim | Explicit unsupported reason; no unwrapped retry |

## UI previews

The task requests native rich previews instead of ASCII/HTML. These tables define the same structural contract and are shown through Kandev rich output at handoff. Labels illustrate grouping, not final untranslated copy.

### UI-01: Execution evidence

Entry: selected session's Execution details. AC-AGENTS-SANDBOX-009.1 through .4.

| Order | Desktop dialog | Phone full-height view |
| --- | --- | --- |
| Fixed header | Execution details / Close | Back / Execution details |
| First group | Process: running, checked time | Same fact and time, full-width row |
| Second group | Launch checks: passed, launch time | Expand named checks in place |
| Third group | Runtime: reported facts / limitations | Same facts with wrapped labels |
| Fourth group | Audit: live/partial/unsigned or verified | Open audit details as next view |
| Final group | Captured policy / next launch | Same sections in one scroller |

The neutral tab indicator remains when live confinement is unverified. Loading and unavailable facts retain their labels. Disconnect makes live process data stale while launch history remains. No success color implies stronger evidence.

### UI-02: Reports and audit

Entry: Execution details / Reports. AC-AGENTS-SANDBOX-010.1 through .5.

| Order | Desktop dialog | Phone full-height next view |
| --- | --- | --- |
| Fixed header | Reports / Close | Back / Reports |
| Summary | Coverage by kind; last successful read | Stacked coverage and age labels |
| Filter | Kind selector and decision filter | Touch picker, same shared filter |
| Body | Event rows with exact scope | Wrapping event cards, visible details action |
| Review | Selected exact records / Review additions | Same action in fixed safe-area footer |
| Verification | Final record / Verify audit / result details | Next view with one scroller and Verify audit |

Unsigned, incomplete, invalid signature, untrusted signer, and valid signature are separate states. A failed refresh preserves old rows with a stale banner. Incomplete/redacted identities explain why learning is disabled.

### UI-03: Typed policy and approval

Entry: executor policy editor or selected session approval. AC-AGENTS-SANDBOX-011.1 through .7.

| Order | Desktop dialog | Phone full-height next view |
| --- | --- | --- |
| Policy editor | Static access / Ask for approval groups | Same groups in one column; typed editor as next view |
| Review header | Operation type / attempt / deadline | Back / operation type, followed by attempt and deadline |
| Scope | TCP host:port, or command/caller/resources, or HTTP origin/method/path | Exact fields wrap; long values expand in place |
| Explanation | One connection, invocation, or request | Same scope warning above fixed actions |
| Fixed actions | Deny / Allow once | Deny / Allow once, each at least 44px |

Expired requests replace actions with their final outcome. Disconnection disables decisions. Read-only users see the same permitted facts without mutation controls. A stale policy draft retains edits and offers explicit reload. Responsive changes preserve the pending request and draft. Network review states that a connection can carry multiple application requests.

Structural requirements: desktop 28px controls; phone/coarse pointer at least 44px targets; fixed heading/Back and decision footer; one internal scroll owner; safe-area clearance; no page overflow. Use existing `SessionExecutionDetails` and approval review as live exemplars, with curated task-switcher header/safe-area conventions. Phone review is a next view, not a stack of sheets. Shared business logic preserves drafts and pending identities across 393/700/767/768px transitions. Icons use existing tokens/currentColor and visible labels. No new decorative motion is required.

## Tests and traceability

New test names below are planned deliverables, not passing evidence. Extend existing suites where they own the contract.

| Criteria | Test location and intended case |
| --- | --- |
| AC-PLUGINS-SANDBOX-002.1-.5 | `internal/plugins/sandbox_provider_v2_test.go`: `TestSandboxV2Compatibility`, `TestSandboxV2ServiceIdentity`; SDK consumer and second fixture |
| AC-AGENTS-SANDBOX-009.1-.4 | `sandboxruntime/evidence_test.go`: `TestObservationFactsRemainIndependent`; execution-status/store tests for stale/sibling/disconnect |
| AC-AGENTS-SANDBOX-010.1-.2 | Plugin `reports_test.go`: `TestTypedAuditCoverage`; host report tests for partial chunks, corruption, and retained data |
| AC-AGENTS-SANDBOX-010.3-.4 | Plugin `audit_verification_test.go`: `TestFinalAuditTrustBinding`; host verification persistence/restart tests |
| AC-AGENTS-SANDBOX-011.1-.7 | Common policy and approval tests: `TestTypedApprovalEnvelope`, `TestLegacyPolicyMeaning`, `TestRuntimeServiceApprovalBinding`; real operation fixtures |
| AC-AGENTS-SANDBOX-010.5, 011.6-.7 | Learning/store tests: `TestTypedLearningPreservesAuthority`; explicit publication/adoption |
| AC-AGENTS-SANDBOX-012.1-.4 and baseline 003/005/006 | `scripts/check-nono-real.py`, `scripts/nono_managed.py`, packaged plugin runtime tests with actual agent transports |

Backend persistence checks cover both SQLite and PostgreSQL, including unavailable prerequisites reported as blockers. Unit tests pair positive cases with stale authority, replay, wrong types, malformed evidence, policy weakening, and sibling isolation. Each work order lists exact commands.

## E2E tests

| Flow | Desktop / mobile-chrome files | Criteria |
| --- | --- | --- |
| Independent facts and disconnect | `execution-details.spec.ts`, `mobile-execution-details.spec.ts` | 009.1-.4 |
| Typed reports, partial/empty/error | `sandbox-reports.spec.ts`, `mobile-sandbox-reports.spec.ts` | 010.1-.2, 010.5 |
| Signature/trust results | new `sandbox-audit-verification.spec.ts`, `mobile-sandbox-audit-verification.spec.ts` | 010.3-.5 |
| TCP/command/endpoint decisions | `sandbox-approvals.spec.ts`, `mobile-sandbox-approvals.spec.ts` | 011.2-.6 |
| Typed policy editor/migration | `sandbox-executor-settings.spec.ts`, `mobile-sandbox-executor-settings.spec.ts` | 011.1, 011.3, 011.5, 011.7 |
| Typed learning and adoption separation | `sandbox-learning.spec.ts`, `mobile-sandbox-learning.spec.ts` | 010.5, 011.6-.7 |

Files are under `apps/web/e2e/tests/task/`, except editor files under `settings/`. Build backend/web before rendered checks. Run suites sequentially with the repository worker guards. Cover keyboard/focus, read-only authority, expiry, long targets, one scroll owner, safe-area controls, and measured phone hit targets. Inspect desktop and phone screenshots. Browser fixtures do not replace real-runtime enforcement tests.

## Work orders

- [x] [Task 01: Migrate the plugin SDK and evaluate pinned Nono 0.81.0](task-01-sdk-runtime.md)
- [x] [Task 02: Add compatible major 2 provider contracts](task-02-provider-protocol.md)
- [x] [Task 03: Show separate process and launch evidence](task-03-runtime-facts.md)
- [x] [Task 04: Expose network and command audit reports](task-04-typed-reports.md)
- [x] [Task 05: Verify finalized audit artifacts against explicit trust](task-05-audit-verification.md)
- [ ] [Task 06: Deliver exact connection approvals and policy envelopes](task-06-network-approvals.md)
- [ ] [Task 07: Deliver confined command invocation approvals](task-07-command-approvals.md)
- [ ] [Task 08: Deliver exact HTTP endpoint approvals](task-08-endpoint-approvals.md)
- [ ] [Task 09: Extend explicit policy learning to supported exact operations](task-09-typed-learning.md)
- [ ] [Task 10: Prove the upgraded host and packaged plugin together](task-10-managed-acceptance.md)

Dependency order is 01 through 10. Tasks 01-05 deliver reporting/status foundations. Tasks 06-09 add typed approvals and learning. Task 10 proves the final pair. No work order authorizes delegation.

## Delivery and verification results

Design validation passed on 2026-10-30:

- `python3 scripts/list-docs.py validate`: 416 decisions and 1818 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- Repository `validateCoverage` preflight: ten work orders, zero reference errors. The temporary checker used one synthetic runtime path to exercise coverage validation; it changed no runtime source.
- All work-order dependency references, 15 new documents' local links, and verification-block shell syntax passed.
- `git diff --check`: passed. All new artifacts are visible in the specification catalog or plan directory.

Task-level results are recorded in each work order. Tasks 01-05 passed their required checks. Tasks 06-10 remain pending. Do not mark work complete while a required real-runtime case is skipped or unsupported. Keep capability refusal when a gate fails and report the blocker.

Leave this package unstaged and uncommitted for review. After a later explicit implementation request, use TDD per work order. Update host PR #4769 and companion PR #2 within existing authorization after their affected checks pass. A production plugin release additionally requires the public compatible SDK dependency and independent package validation. No merge is authorized.

## Risks

The candidate's network request identity differs from file requests. Conditional audit metadata is not universal. Exact argv/path semantics can differ across platforms and runtime versions. Endpoint mediation requires per-attempt TLS trust with no global side effects. Runtime event truncation or redaction can prevent learning. SDK extraction and protocol-major changes need independent consumer checks. A new runtime's security fixes and behavior changes require artifact-specific acceptance, not a version-string edit.

## Implementation authorization (2026-11-02)

The user explicitly requested implementation of this package. Implementation follows the ten sequential work orders in the primary session. The design checkpoint above is historical. Existing branch/PR authorization remains; do not merge or change retained instances.

## Foundation delivery checkpoint (2026-11-02)

Tasks 01-05 are complete. The host foundation, independent process/launch/runtime facts, typed audit reports, and finalized audit verification are implemented. Host UI checkpoint `4970f79f3724` and provider key-classification checkpoint `c45dae9d` are pushed to their draft PRs. Task 05 passes its persistence/restart and native-verifier checks, 13 controller/current-trust tests, ten desktop/phone browser journeys, seven-locale checks, scoped lint, documentation checks, and builds. Explicit verification RPCs use the 30-second job bound; ordinary reads retain five seconds and shorter caller/wire deadlines win. Full plugin and packaged-runtime race suites pass. Tasks 06-10 remain pending. Current-pair managed acceptance is not complete. Moving main introduced PR conflicts; rebase and integration checks precede the next work order. Main :9998 and retained instances remain unchanged. No PR merge is authorized.

## Rebase integration checkpoint (2026-11-03)

The host now contains verified main `c060076d6723a611ebc260371aa80b555442e589`; all 25 initiative commits remain. Cancellation during plugin lifecycle writes and sandboxed Copilot reset environment coherence have RED/GREEN coverage. Final affected race suites, SQLite/PostgreSQL owning-store and previous-stable upgrade checks, SQL guard, zero-issue Go lint, 13 Audit UI tests, typecheck, ten fresh desktop/phone browser journeys, localization, and documentation checks pass. Task 05 records the commands and limits. The private PostgreSQL fixture was restarted on its task-owned Unix socket; main and retained instances were preserved.

Task 06 remains pending. Preliminary native probes require `--isolate net` to close a same-proxy-port alternate-address bypass. The full packaged network acceptance matrix remains required before capability activation.

## Task 06 execution (2026-11-03)

Task 06 is in progress. Host rebase checkpoint `c87e576f6018` and plugin SDK pin `f35f5aa87` are pushed. The public standalone consumer, plugin validation/package verification, and rebuilt packaged host/runtime checks pass. The first slice requires network namespaces for new Nono proxy launches, with a permanent same-proxy-port alternate-address regression and permitted-work control. The legacy relay remains in place. Schema 2, exact operator approvals, UI, and the full network conformance gate remain unfinished.

The first Task 06 slice is complete: new proxy launches require a network namespace. Compiler and actual-runtime RED/GREEN tests close the alternate-address proxy-port bypass while preserving permitted work. Process-scoped namespace-refusal tests fail closed. Full provider/package validation, the independent public SDK consumer, uncached host/package runtime checks, SQLite/PostgreSQL policy tests, and public docs validation pass. The work order records limits and exact commands. Typed policy/approval implementation continues; Task 06 is not complete.


## Rebase and package-isolation checkpoint (2026-11-04)

The host contains main `74abd2c92887e3b7b7d6a14eb803067e4e0f9565` and all 28 initiative commits. The integration captures selected built-in MCP routes and keeps separate registries behind the private dynamic transport. Native configuration, ACP session requests, exact permission checks, restart retention, and helper capability refusal have regression coverage. Actual packaged Nono tests pair permitted package access with cross-package and unselected-route denials. Full core/policy/runtime and affected integration checks, final regressions, SQLite/PostgreSQL conformance, SQL guard, zero-issue scoped lint, typecheck, 76 frontend tests, fresh desktop/phone approval journeys, and docs/spec checks pass. The work order records commands and limits.

Task 06 remains in progress. Native service actors, exact TCP decisions, migration/editor UI, full network conformance, and Tasks 07-10 remain unfinished. Current Nono capabilities remain schema 1. Current-pair managed acceptance is still required. The host and provider PRs remain drafts. Main `:9998` and retained instances remain unchanged.

## Draft discovery integration checkpoint (2026-11-04)

The owned branch contains main `5d6adcbca18231a5aa3726ca9ee642bfa255cd4a` and all 29 initiative commits. Conflict fixes retain draft model discovery and grouped catalogs together with execution settings. Full affected backend race tests, zero-issue scoped Go/Oxlint checks, typecheck, localization, 59 frontend tests, four fresh desktop journeys, and three fresh phone journeys pass. The work order records interrupted-build recovery and final evidence. Task 06 remains in progress. Native TCP actors/decisions, editor/migration UI, complete conformance, and current-pair managed acceptance remain required. Nono remains on schema 1. Both PRs remain drafts, with no merge or retained-instance changes.
