---
created: 2026-10-22
status: done
requirements:
  - REQ-AGENTS-SANDBOX-001
  - REQ-AGENTS-SANDBOX-002
  - REQ-AGENTS-SANDBOX-003
  - REQ-AGENTS-SANDBOX-004
  - REQ-AGENTS-SANDBOX-005
  - REQ-AGENTS-SANDBOX-006
  - REQ-AGENTS-SANDBOX-007
  - REQ-AGENTS-SANDBOX-008
  - REQ-PLUGINS-SANDBOX-001
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-005
system_design:
  - ../../specs/agents/system-design/sandbox-policies.md
  - ../../specs/plugins/system-design/sandbox-provider-capability.md
  - ../../specs/tasks/system-design/bootstrap-attempt-recovery.md
legacy_specs: []
---

# Implementation plan: Nono sandbox integration

## Overview

Deliver optional sandbox policies and Nono provider services above the core wrapping in PR #4538.
First preserve runtime evidence and repair generic failed-bootstrap recovery.
Then add the optional contract, policy authority, dedicated provider, approvals, MCP routing, and operator learning.
The user authorized implementation after reviewing the package. Tasks 01, 02, 03, and 08 are complete.
Tasks 04 through 07 and 10 now have completed component and actual managed-agent acceptance. Task 09 has completed real acceptance and draft-PR delivery. Current-policy attestation remains explicitly unsupported, with unknown status.

Own branch: `feature/integrate-nono-sandb-7tq`.
Verified base: `main` at `1eb53576f496759b58ae73d4cba7bd96cbc91288`.
[PR #4538](https://github.com/kdlbs/kandev/pull/4538) merged as `76c9ada7baaa6b15bf814aec2ef093cc7dc07610`.
The user authorized the base change after that merge. [Draft PR #4769](https://github.com/kdlbs/kandev/pull/4769) targets `main` and identifies #4538 as its merged prerequisite.
The original design commit was replayed on this base. The parent branch and worktree remain unchanged.

## Scope

### In scope

Core wrapping compatibility, explicit policy selection and authority, lifecycle ownership, verified approval transport, reports, learning, MCP connectivity, and enforcement evidence.
The generic recovery amendment covers Task 04 without coupling task status to Nono.
The Execution section, per-session Execution details, and Dockview/mobile indicator use the common provider contract.
Generic wrappers remain available without plugins. A wrapper indicator never asserts sandbox enforcement.

### Out of scope

Remote sandbox support, universal platform support, automatic policy learning, and PR merges.
The main instance on 9998, retained instances, and deprecated #3812 task remain untouched.

## Assumption check

Confirmed: basic wrapping stays in core, plugins are optional, production providers use dedicated repositories, and the design checkpoint precedes implementation.
Verified: the retained authentication/bootstrap/queue sequence and the pinned file-approval exchange appear in [investigation.md](investigation.md).
Proposed for review: authenticated-human-only structured enforcement, immutable managed policy revisions, and a protected per-attempt callback capability.
Verified in component tests: actual Nono callback isolation, file decisions, and a Kandev MCP tool call through a per-attempt relay.
Managed Nono discovery, ACP/native file and MCP work, PTY input/resize, reviewed restart, cancellation, detached-child cleanup, scoped deletion, and paired negative enforcement checks passed. Settings and indicators have desktop/phone fixture checks. macOS and non-file approvals remain unsupported.
These are explicit implementation gates. The package does not label them supported.

## Design inputs

- [Agent requirements](../../specs/agents/requirements/sandbox-policies.md) and [design](../../specs/agents/system-design/sandbox-policies.md).
- [Plugin requirements](../../specs/plugins/requirements/sandbox-provider-capability.md) and [design](../../specs/plugins/system-design/sandbox-provider-capability.md).
- [Task recovery amendment](../../specs/tasks/requirements/task-launch-failure-recovery.md#proposed-bootstrap-recovery-amendment) and [design](../../specs/tasks/system-design/bootstrap-attempt-recovery.md).

## Ownership and technical approach

Agents owns effective launch policy and operator workflows.
Plugins owns the optional ABI and installation lifecycle.
Tasks extends its existing launch-recovery requirements at the failed-bootstrap boundary.
The [ADR](../../decisions/2026-10-22-optional-sandbox-provider-boundary.md) records the boundary alternatives.

Use `internal/common/launchwrapper`, hostutility, lifecycle, agentctl process/utility, and existing MCP strategies as integration points.
Add core policy storage and immutable attempt snapshots through existing persistence and migration patterns.
Add the optional plugin SDK/protobuf/manifest contribution without changing `ExecutorProviderPlugin` semantics.
Production Nono implementation belongs in the proposed `kdlbs/kandev-plugin-nono` repository.
The dedicated repository is a later work-order dependency. Do not create a nested repository here.

Task 10 owns the vertical Execution UI slice after the policy and provider lifecycle prerequisites.
It captures safe launch snapshots, projects attempt-bound evidence, and delivers settings, details, and indicators together.
Task 08 extends that shared detail surface with denial review and learning. It does not create competing policy selectors.
Use `ProfileFormFields`, `CommandPrefixField`, `session-tab.tsx`, and `session-mobile-top-bar.tsx` as current integration points.
New snapshot/projection types and tests are proposed deliverables. Provider metadata supplies generic capabilities and canonical grants.

| Provider/transport | Intended behavior | Required evidence | Unsupported fallback |
| --- | --- | --- | --- |
| Generic prefix, plugins disabled | Existing core behavior | Parent fixture regressions | Existing typed refusal |
| Nono 0.80.0, Linux Local/Worktree ACP | Piped protocol under explicit policy | Discovery, tools, restart, cleanup | Fail before launch |
| Nono native Pi terminal | Kandev outer PTY plus Nono supervisor | Input, resize, signals, resume | Refuse unsupported transport |
| Nono Linux file approval | Verified webhook protocol plus isolated callback | Real grant/deny/deadline and forgery controls | Deny |
| Command/network endpoint approval | Not initially advertised | Separate actual runtime evidence | Explicit unsupported denial |
| macOS static policy | Conditional future activation | Actual Seatbelt acceptance | Unsupported until tested |
| Remote/Windows sandbox | Excluded from v1 | No host-only inference | Unsupported |

## UI preview

The task's rich-output preview presents these layouts. Text below preserves the review contract without ASCII or HTML diagrams.
Labels are illustrative localized copy. Control order, authority, and phone composition are requirements.

| View | Desktop | Phone | Required states |
| --- | --- | --- | --- |
| UI-01: Execution settings | Visible Execution section: Command wrapper editor/validation/preview, then Sandbox provider/default/summary, then Save | Same group order in one column, bottom picker for executor/policy, visible source and Save | Loading, no plugin, unresolved executor, unavailable, invalid revision, read-only, saved |
| UI-02: Approval | Session card: operation, resource, policy, deadline, Deny, Allow once | Full-height operation review, fixed Deny and Allow once, Back | Pending, submitting, denied, expired, provider unavailable |
| UI-03: Learning | Denial list, completeness, selected access diff, persistence scope, Create revision | Full-height denial detail, next view with diff and scope, fixed Create revision | Empty, incomplete evidence, stale revision, ceiling violation, saved |
| UI-04: Recovery | Session-specific error, blocked queue count, Retry, history | Full-width recovery card, visible queue and Retry controls | Authentication required, retry pending, failed retry, recovered |
| UI-05: Execution details | Session action opens dialog: status/time, This attempt, Protection scope, Next launch, Approvals, Denials | Labelled Execution status opens full-height surface with fixed Back/title and one scroll body | Configured, starting, active, failed, unknown, stopped, not configured, incomplete report |
| UI-06: Session indicator | One button beside the Dockview session title, keyboard/pointer activation opens UI-05 | Selected-session status control with 44px target, same UI-05 data | Neutral wrapper, confirmed active shield, failure warning, neutral pending/unknown/stopped |

UI-01 maps to SANDBOX-007.1-6. UI-05 and UI-06 map to SANDBOX-008.1-8.
Before this change, Command prefix is inside Advanced options. The proposed Execution section makes it directly discoverable.
The settings summary orders readable locations, writable locations, network, Kandev MCP, and mandatory restrictions.
UI-05 keeps the current attempt separate from next-launch defaults. A changed default never repaints an existing attempt.
UI-06 shows one status for one session. A wrapper-only session cannot receive an active shield.
A plain session can omit the icon but retains an Execution details action. No task-wide security badge is introduced.

Empty and failure composition: keep the wrapper group editable without plugins. Keep unavailable sandbox selections visible with their blocker.
Unknown evidence replaces the live claim with a reason and last observation time. It does not erase captured configuration.
The details view labels protection boundaries and offers existing approval/report actions only when supported and authorized.
These grouping, navigation, state, and scope rules are structural. Sample labels are illustrative and must use localization.

Use `MobilePickerSheet` for temporary choices and `session-task-switcher-sheet.tsx` geometry for safe-area behavior.
A full-height review has one internal scroll owner and fixed actions outside that scroll area.
Use dynamic viewport height, keyboard focus return, visible Back, and at least 44px phone targets.
Desktop controls retain the repository's 28px density. Shared state survives breakpoint changes.
All new copy needs seven locales. No new rendered UI is implemented during this turn.

## Tests and traceability

All names below identify planned tests. Each work order lists exact commands and owned files.
Each criterion listed in work-order frontmatter needs a behavioral assertion, not only DTO equality.

| Criteria | Planned evidence |
| --- | --- |
| SANDBOX-001.1-4, PLUGINS-SANDBOX-001.1-6 | `sandbox_provider_test.go`: `TestSandboxProviderCompatibility`, malformed artifacts, disable/crash, plugins-disabled core control |
| SANDBOX-002.1-6 | `sandbox/policy_test.go`: `TestSandboxPolicyPrecedence`, `TestSandboxAuthority`, imports, races, SQLite/PostgreSQL persistence |
| SANDBOX-003.1-2, .6 | `sandbox_lifecycle_test.go`: `TestSandboxLifecycle`, `TestSandboxTransport`, real owned subprocesses and retained snapshots |
| SANDBOX-003.3-5 | `sandbox/approval_test.go`: `TestSandboxApproval`, forged identities, stale decisions, ceiling rejection, cancellation and deadline |
| SANDBOX-004.1-5 | `sandbox/report_test.go`, `learning_test.go`: incomplete reports, immutable provenance, suppressions, optimistic concurrency |
| SANDBOX-005.1-4 | `sandbox/endpoints_test.go`: `TestSandboxMCP`, `TestSandboxEndpoint`, dynamic rotation and denied alternate routes |
| SANDBOX-006.1-2 | Desktop/mobile policy and approval specs, localized states, keyboard and touch geometry |
| SANDBOX-006.3-4 | `check-nono-real.py`: actual pinned runtime and packaged plugin acceptance |
| SANDBOX-007.1-6 | `profile-execution-section.test.tsx`, `policy_test.go`: prefix preservation, plugins-disabled save, executor context, coexistence, source and ceiling |
| SANDBOX-008.1-2, .4, .8 | `execution_snapshot_test.go`: `TestExecutionSnapshot`, `TestExecutionEvidence`. `execution-status.test.ts`: stale/foreign events, reconnect, legacy unknown, redaction, current/next separation |
| SANDBOX-008.3, .5-7 | `session-execution-indicator.test.tsx`, `session-execution-details.test.tsx`: truthful icons, protection scope, capability states, keyboard, touch and tab event isolation |
| TASK-LAUNCH-FAILURE-RECOVERY-005.1-5 | `bootstrap_attempt_recovery_test.go`: `TestBootstrapAttempt`, pending-launch controls, same-session retry, sibling isolation, exact queue dispatch |

## E2E tests

Task 02 owns `tests/task/bootstrap-attempt-recovery.spec.ts` and its `mobile-` counterpart.
Task 06 owns `tests/task/sandbox-approvals.spec.ts` and its phone counterpart.
Task 10 owns `tests/settings/execution-settings.spec.ts`, `tests/task/execution-details.spec.ts`, and their `mobile-` counterparts.
Cover a custom wrapper with plugins disabled, two differently named fixture providers, and wrapper-plus-sandbox composition.
Cover two sibling sessions with different policy states, stopped/legacy history, stale events, reconnect, and next-launch changes.
Check redaction, focus return, tab actions, long names, phone keyboard, and 44px targets at rendered desktop and phone sizes.
Task 08 owns `tests/settings/sandbox-policies.spec.ts` and its phone counterpart for report review, learning, and explicit learned-revision selection.
Fixture providers prove generic UI contracts only. Task 09 still requires real provider/runtime evidence.
Use Chromium and the configured mobile-chrome project with one worker and no retries for delivery evidence.
No broad suite or generic review work order is added.

## Work orders

- [x] [Task 01: Pin the runtime and build conformance evidence](task-01-runtime-contract.md)
- [x] [Task 02: Recover failed bootstrap attempts without losing queued work](task-02-bootstrap-recovery.md)
- [x] [Task 03: Add the optional sandbox provider contract](task-03-plugin-capability.md)
- [x] [Task 04: Persist and enforce policy selections](task-04-policy-authority.md)
- [x] [Task 05: Implement Nono in its own repository](task-05-nono-provider-lifecycle.md)
- [x] [Task 06: Connect verified approvals to operator decisions](task-06-approval-channel.md)
- [x] [Task 07: Permit only the current Kandev MCP route](task-07-mcp-connectivity.md)
- [x] [Task 10: Deliver Execution settings and session visibility](task-10-execution-surfaces.md)
- [x] [Task 08: Deliver denial review and explicit learning](task-08-operator-policy-learning.md)
- [x] [Task 09: Prove real Nono acceptance and prepare the follow-up PR](task-09-acceptance-delivery.md)

Execute sequentially in listed order: 01, 02, 03, 04, 05, 06, 07, 10, 08, 09.
Task IDs remain stable after adding Task 10. Dependency lists state hard prerequisites, not permission to delegate.
The user retains model and implementation control in this primary session.

## Delivery

After a later explicit implementation request, complete work orders and record every required result.
Use a fresh disposable instance and source-built binaries. Supply any real provider login privately for that instance only.
Missing credentials or unsupported hosts block affected acceptance. Never borrow retained login files.
Record companion plugin commit/package digest and its independently validated release artifact.
Public documentation changes belong to implementation. This design-only package does not advertise unshipped behavior.

Before opening the new draft PR, verify the current remote `main` and the merged prerequisite #4538.
Integrate later main changes only on this feature branch and rerun affected checks.
Use `gh pr create --draft --base main --body-file <prepared-file>` after authorized commit/push and task checks.
Use the repository PR template. Identify #4538 as the merged prerequisite and distinguish host and plugin deliverables.
Never commit or push to the former parent branch. Do not merge the follow-up PR.

## Verification results

Design validation passed on 2026-10-22:

- `python3 scripts/list-docs.py validate`: 396 decisions and 1692 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `.github/scripts/pr-docs.cjs::validateCoverage`: nine work orders, zero reference errors.
- Local Markdown targets and work-order dependency links: passed.
- STE structural check across new artifacts: zero hard findings with synonym-rotation warnings disabled.
- `git diff --check`: passed.

The coverage check used `/tmp/kandev-nono-design-evidence/validate-coverage.cjs` and one synthetic runtime path to force validation.
It did not modify runtime source or publish a GitHub check.
The STE check retains advisory passive-voice findings and does not claim formal dictionary compliance.
The original nine implementation work orders remained pending at that checkpoint.

Execution UI amendment on 2026-10-23: ten work orders now remain pending.
Amendment validation passed:

- `python3 scripts/list-docs.py validate`: 401 decisions and 1727 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `.github/scripts/pr-docs.cjs::validateCoverage`: ten work orders, zero reference errors.
- Local Markdown targets, work-order dependencies, and Task 10 existing source paths: passed.
- STE structural checks across changed artifacts: zero hard findings, with synonym rotation disabled and passive voice advisory.
- `git diff --check`: passed.

The amendment remains unstaged and uncommitted for review. No production or permanent test code changed.
The earlier Nono mock seed tests only inherited generic wrapping. They do not verify these proposed surfaces.
Production implementation, permanent tests, and full Nono acceptance have not run.
The temporary runtime probes and historical incident evidence are recorded separately in [investigation.md](investigation.md).

## Risks

Callback isolation and nested PTY behavior require actual negative evidence before activation.
The current Nono documentation and pinned source disagree about webhook availability.
Kernel support, same-user trust, policy inheritance, and missing audit data constrain support claims.
A successful replacement conversation cannot close the original recovery acceptance item.


## Current implementation checkpoint (2026-10-27)

Tasks 01 through 08 and 10 are complete for the supported Linux amd64 Local/Worktree matrix. Task 09 records real managed acceptance and draft-PR delivery. Earlier pending statements above are historical checkpoints. The runtime feature remains off in shipped profiles. Nono supplies no accepted current-policy attestation, so actual status remains unknown. Both PRs remain drafts. The user test endpoint remains stopped.

## Implementation walkthrough

The [host and Nono walkthrough](walkthrough.md) explains ownership, the six provider RPCs, policy selection, approvals, MCP connectivity, restart, cleanup, evidence limits, and future adapters. It records the inspected commits and links directly to both implementations.
