---
created: 2026-11-12
status: done
requirements:
  - REQ-AGENTS-SESSION-AUTHENTICATION-001
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007
system_design:
  - ../../specs/agents/system-design/session-authentication-recovery.md
legacy_specs: []
---

# Fix plan: Cursor startup authentication recovery

## Overview

Preserve explicit provider authentication failures through ACP startup, durable recovery records, and the shared desktop and phone recovery card.
Issue [#5293](https://github.com/kdlbs/kandev/issues/5293) is assigned to `carlosflorencio`.
GitHub readback confirmed the assignment on November 12, 2026.

Agents owns this package because it interprets provider evidence and owns recovery presentation.
Tasks retains storage, admission, workspace, and native conversation ownership.
The [authentication requirement](../../specs/agents/requirements/session-authentication-recovery.md) defines the missing authentication-specific behavior.
Existing [recovery criteria](../../specs/agents/requirements/session-recovery-failures.md) remain authoritative for the shared card, history, and attempt isolation.
The [design](../../specs/agents/system-design/session-authentication-recovery.md) covers the complete correction.

Implemented sequentially: operation evidence and lifecycle classification, durable recovery projection, then presentation and browser proof.
Work orders ran sequentially; delegation was not used.

## Confirmed evidence

Investigation base: `94e35cd2234608185643c91e9f1aeebf1f1956ca`.
The canonical issue has no comments or image attachments.
Its private screenshot was deliberately omitted from the public report.
The reported macOS incident was not reproduced against a native Cursor account.

Source tracing confirms:

1. ACP `session/new` returns authentication-required code `-32000`.
   `maybeEmitAuthRequired` emits a connection-scoped event only when cached methods exist.
2. Agentctl `handleWSNewSession` sends `err.Error()` without typed session-failure metadata.
   Client `createSessionRequest` constructs another formatted string, losing typed ACP identity.
   `load`, `resume`, and `reset` have related session-operation response boundaries.
3. `bootstrapFailureFor` has no authentication recognition branch.
   The existing `authentication_required` cause and fixed safe text are already supported by its projection helpers.
4. `handleAgentStartFailed` detects authentication from the original diagnostic.
   Its authentication projection update is restricted to `ManagedRuntimeStartupError`.
5. `createRecoveryStatusMessage` checks only `isAuthError(data.ErrorMessage)`.
   Generic safe text therefore produces `is_auth_error: false`, hides methods, and selects ordinary fresh-start copy.
6. `ActionMessageDetails` follows that flag. The mounted `SessionRecoveryCard` uses that component too.
   Durable typed causes need to retain generic guidance while matching transcript metadata is unavailable.

### Temporary reproduction

Command, from the repository root:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -p 1 ./internal/agent/runtime/lifecycle ./internal/orchestrator -run '^TestRepro5293' -count=1 -v)
```

| Temporary test and input | Observed result |
| --- | --- |
| `TestRepro5293BootstrapAuthenticationClassification/typed_acp_error`: wrapped `acp.NewAuthRequired(nil)` | `SafeCode() == unknown`; generic safe detail |
| Same test, `current_stream_projection`: current flattened session error | Same classification loss |
| `TestRepro5293RecoveryIgnoresStructuredAuthentication/failure_code`: explicit `auth_required`, generic display text | `is_auth_error == false`; `Start fresh session` |
| Same test, `bootstrap_cause`: normalized `authentication_required` cause, generic display text | Same false metadata and action |

Both tests failed for the intended assertions, across four subcases.
Their temporary files were removed after completion.
The flattened-string subcase documents the current boundary loss; permanent coverage must cross the repaired typed response instead of trusting arbitrary strings.

Existing adapter baseline also passed:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -p 1 ./internal/agentctl/server/adapter/transport/acp -run '^TestMaybeEmitAuthRequired_' -count=1 -v)
```

All four existing tests passed: auth event, non-auth error, plain transport error, and empty-method behavior.
No production code, permanent tests, credentials, or live task state changed during diagnosis.

## Scope

### In scope

- Typed ACP session-operation authentication evidence and bounded method snapshots.
- Initial startup and supported restoration, including nested Go wrappers.
- Durable bootstrap cause, auth metadata, history, and provider-aware guidance.
- Empty, missing, filtered, and execution-cleaned authentication method cases.
- Early connection-scoped events, exact-attempt guards, and native identity preservation.
- Localized active-card guidance, existing login options, safe details, and explicit retry.
- Deterministic backend and desktop/phone browser coverage with disposable fixtures.

### Out of scope

- Changing or diagnosing the user's Cursor credentials, or accessing the reported live incident.
- A new login continuation service, automatic login, shell commands derived from provider prose, or static Cursor methods invented from the incident.
- New provider fallback policy, retry budgets, delivery ownership exceptions, tables, flags, or SQL migrations.
- Rewriting historical unknown failures or broad error-classifier refactoring.
- Repository/MCP authentication repair, Git warning changes, and unrelated issues #5128 or #5018.

## Technical approach

### Work order 01: Operation evidence

Add `types.SessionOperationFailure` for the allowlisted authentication projection.
Attach it through optional `ErrorPayload.Details.session_failure` at the actual ACP session-operation failure boundaries.
Read methods through a narrow optional adapter accessor, with bounds and sanitization from the design.
Keep identity-free ACP `agent_capabilities` and connection-scoped `auth_required` events supplemental and ephemeral because neither has native session identity; persist the matching typed operation response as the durable authentication authority.
Hold MCP attachment attempt/configuration preambles in the session-operation context. ACP emits the preamble before result evidence only after success or a non-authentication result; a typed authentication rejection emits no MCP attachment events because the failed first `session/new` has no committed native generation.
Decode the matching response into a typed client error.
Teach lifecycle `bootstrapFailureFor` to retain the existing authentication cause and fixed detail.
Carry the safe methods transiently with the error; never put methods or raw RPC data into `AgentErrorCause`.

### Work order 02: Durable recovery

Carry the established auth decision through `handleAgentStartFailed` without the managed-runtime-only restriction.
Use a common structured-evidence predicate for recovery metadata and action selection.
Preserve the task bootstrap category separately from routing and cause codes.
Persist the safe method snapshot before exact-execution cleanup.
Audit `persistLastAgentError`, executor `buildBootstrapLastAgentError`, transcript metadata, and hydration copies.
Add repository-reopen coverage and startup-event barriers without weakening delivery ownership.

### Work order 03: Presentation and end-to-end proof

Add provider authentication copy and a durable-cause fallback to the existing active card.
Keep method guidance useful when a method cannot execute after cleanup.
Use the existing live-owned authenticate path for protocol methods. Structured terminal commands stay copy-only unless a terminal is validated for the same executor environment and live owner.
Correct reused authentication controls to shared desktop/touch sizing.
Add a private ACP fixture that returns the real `session/new` error through agentctl.
The fixture must also support successful creation after explicit login state changes and preserve a valid identity on restore failure.
New copy goes through all shipped catalogs.
Keep explicit retry eligible when identity-free startup authentication projects as `WAITING_FOR_INPUT`; generic waiting sessions, non-start authentication failures, dismissed causes, and sessions with native identity remain ineligible.
Base retry eligibility on canonical current-session native identity and explicit resume-token evidence. Missing transcript metadata is unknown, not proof that a session can be reset.
Apply authentication labels and eligibility within the shared recovery-choice pipeline, preserving inspection prerequisites, eligible read-only workspace restore, branch recovery, history continuation, and guard filtering.

### Compatibility matrix

| Provider/transport shape | Evidence and behavior | Verification and fallback |
| --- | --- | --- |
| Cursor ACP, initial `session/new` | ACP `-32000` becomes `authentication_required`; restart guidance; no invented native ID | Cursor-identity backend fixture; provider-neutral real-pipeline browser fixture |
| Other ACP providers using the same operations | Same typed ACP semantics, provider identity from configured session | Parameterized API/client/lifecycle tests |
| Supported ACP load/resume with valid native identity | Retain auth cause and existing identity; eligible explicit Resume | Restoration integration test; never invoke fresh creation as a fallback |
| Empty or unavailable methods, or failed execution already released | Keep auth classification and generic guidance | Backend snapshot/cleanup test and metadata-free mounted-card test |
| `cursor_login` without a usable live method control | Preserve safe method identity and generic guidance; never treat the ID as a shell command | Method projection, existing authenticate delegation, and UI tests |
| Older agentctl without typed optional evidence | Existing compatibility behavior; no new arbitrary-string parser | Missing-field tests; safe unknown remains possible when evidence is unavailable |
| Non-ACP/native/passthrough or unrelated operation | Existing classification and controls | Negative tests; no inferred support |
| Malformed, stale, cancelled, terminal, or foreign evidence | Reject the projection or preserve the stronger existing outcome | Validation and ownership barrier tests |

## ASCII UI preview

### UI-01: Task Chat, failed native session creation

Current metadata behavior, supported by source and the temporary reproduction:

```text
The agent could not start
The bootstrap operation could not be completed.
[Start fresh session] [Restore read-only workspace]
> Technical details: bootstrap; unknown
```

Proposed desktop content order:

```text
Cursor authentication is required
Sign in to Cursor on the execution host, then restart the session.
[Supported login guidance, or generic login guidance]
[Restart session] [Restore read-only workspace, when eligible]
> Technical details: Start; authentication_required
```

Proposed phone content order:

```text
Cursor authentication is required
Sign in on the execution host.
Restart after authentication.
[Login guidance                         ]
[Restart session                       ]
[Restore read-only workspace, if valid  ]
> Technical details
```

These previews describe structure; copy is illustrative and must be localized.
Preserve the shared amber card, existing action admission, confirmation policy, and valid workspace-restoration eligibility.
The active card stays at the composer location; the transcript keeps one compact dated entry without duplicate controls.
The card owns expanded-content scrolling with its existing `50dvh` bound.
The transcript remains a sibling scroll region. No nested diagnostic scroller or horizontal document overflow is introduced.

### UI-02: Variants and progress

With usable methods, show their supported login guidance before retry.
With no methods, a protocol-only method after teardown, or unloaded transcript metadata, retain generic authentication guidance.
Never show a working login button for a released execution.
During retry, show the existing live pending status and disable equivalent controls.
If retry fails, retain its new owned cause and prior dated history.
Only authoritative readiness retires matching recovery controls.
For supported restore, show eligible Resume instead of implying that all authentication failures require conversation reset.

UI-01 and UI-02 map to authentication criteria .2-.4 and .6, plus recovery criteria 006.13, .17, and .34.

## Tests

| Criteria | Planned regression evidence |
| --- | --- |
| Authentication .1, .5 | `session_authentication_test.go` in agentctl API: `TestSessionAuthenticationFailureProjection`; client: `TestSessionOperationAuthenticationError`; lifecycle: `TestBootstrapAuthenticationCause`; process/lifecycle stream tests keep identity-free startup controls outside retained delivery while retaining checks for identity-bearing events |
| Authentication .2-.4 | Orchestrator `startup_authentication_recovery_test.go`: `TestStartupAuthenticationRecovery`; `recovery_authentication_test.go`: `TestRecoveryAuthenticationWithoutDiagnosticText` |
| Authentication .3, .5; recovery 006.5, .24-.26 | `TestStartupAuthenticationRecoveryRepositoryReopen`, `TestAuthenticationStartupOrdering`; existing bootstrap successor-fence coverage |
| Authentication .4; recovery 007.1, .2, .5 | Fresh failure without native ID, supported restore retaining its token, cancellation, and stale-attempt integration cases |
| Authentication .2, .6; recovery 006.17, .34 | `session-authentication-recovery.test.ts`, `session-recovery-card-authentication.test.tsx`, and method-panel component coverage |

Each implementation work order starts with failing regression assertions.
Do not replace existing negative or ownership scenarios to obtain a passing fixture.

## E2E tests

Add `e2e/helpers/session-authentication-recovery.ts` beside the shipped startup-process fixture pattern.
Use a private synthetic ACP process, disposable state, and real backend persistence.
Browser transport interception alone cannot prove classification or a successful retry.

| File and project | User outcome and acceptance |
| --- | --- |
| `tests/session/session-authentication-recovery.spec.ts`, `chromium` | Actual startup auth failure, protocol and structured terminal methods, copy-only terminal guidance, metadata/cause agreement, navigation/reload, explicit post-login retry; authentication .1-.4 |
| Same desktop spec plus backend repository-reopen test | Reconstruction without the original error/cache; authentication .3; restart proof must read persisted state |
| `tests/session/mobile-session-authentication-recovery.spec.ts`, `mobile-chrome` | Same guidance and explicit retry by touch, 44-pixel targets, wrapped details, focus and overflow; authentication .2, .4, .6 |

Include privacy sentinels and a non-auth negative case.
The synthetic provider cannot establish the state of a real Cursor account.
When a disposable native Cursor smoke test is available, record its exact version and supported login sequence separately.

## Work orders

- [x] [Task 01: Preserve ACP authentication evidence](task-01-operation-evidence.md)
- [x] [Task 02: Persist authentication recovery](task-02-durable-recovery.md)
- [x] [Task 03: Show actionable authentication guidance](task-03-recovery-guidance.md)

All three work orders are complete, including the focused corrections for identity-safe retry choices, shared recovery actions, and terminal-auth safety.
Work orders remain sequential: 01 -> 02 -> 03.
No subagents, additional Kandev tasks, or extra sessions were used.

## Companion records and documentation

The completed startup explanation, startup process diagnostics, and session error UI packages remain historical implementation records.
This package adds no changes to their completed work orders or recorded test counts.
The draft blocked-session guidance package handles history continuation and is not a dependency of this authentication correction.

Work order 03 updated authentication recovery guidance in `docs/public/agents-and-profiles.md`.
The public section distinguishes provider account login from repository/MCP credentials and identifies the execution host.
It does not prescribe `cursor_login` as a CLI command or claim unverified account repair.

## Verification results

- Diagnosis: both temporary regression tests failed at the reported boundaries; four failing subcases. Temporary files removed.
- Existing adapter baseline: four tests passed.
- `python3 scripts/list-docs.py validate`: passed; 450 decisions and 1997 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: passed; 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Local `.github/scripts/pr-docs.cjs` `validateCoverage` preflight: current documentation diff is exempt.
  A representative runtime change validates all three work orders, requirement/criterion references, and their shared design without errors.
- `node --test scripts/validate-public-docs.test.mjs`: passed; 63 tests.
- `node scripts/validate-public-docs.mjs`: passed; 55 published pages.
- `python3 scripts/list-docs.py validate`: passed; 450 decisions and 1997 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed after implementation and documentation updates.
- Focused UI tests: 3 files and 12 tests passed; web typecheck, full lint, and i18n check passed.
- Review regressions passed for retained native identity without transcript metadata, unknown identity eligibility, shared branch/history/workspace choices, inspection prerequisites, and copy-only terminal auth methods.
- Focused backend authentication regression passed; the three-package backend race suite and changed-file Go lint passed.
- Desktop Chromium and mobile Chrome recovery E2E specs passed against freshly built backend and web artifacts after the review corrections.
- Task 03 records exact commands and results.
- Native Cursor login and successful native account repair were not attempted; they remain outside this implementation environment and scope.

## Risks

- Startup events can precede native identity; fixing this by weakening durable ownership would create a separate defect.
- Cached methods can disappear during teardown. Classification and fallback guidance must survive without them.
- A protocol-only login method cannot run through a released process. The UI must preserve useful guidance without promising a live authentication control.
- The incident does not establish why Cursor required login or whether the user's native login flow succeeds.
- Shared ACP paths require explicit restore and non-auth coverage; auth codes must not become HTTP statuses or missing-native-session evidence.
