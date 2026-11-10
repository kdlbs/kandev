---
created: 2026-11-10
status: done
requirements:
  - REQ-AGENTS-CLAUDE-SESSION-LIMIT-001
  - REQ-AGENTS-CODEX-USAGE-LIMIT-001
system_design:
  - ../../specs/agents/system-design/claude-session-limit-classification.md
  - ../../specs/agents/system-design/codex-usage-limit-classification.md
legacy_specs: []
---

# Implementation Plan: Office Mid-Run Quota Classification

## Overview

Repair the confirmed classifier gaps in [issue #5222](https://github.com/kdlbs/kandev/issues/5222).
Extend the existing Claude and Codex limit contracts through one sequential work order.
Keep recovery policies and failure accounting unchanged.

Agents owns this repair because its shared classifier defines the provider evidence consumed by Office and dynamic profiles.
Office owns routing and agent-failure accounting, which already distinguish handled provider failures from terminal agent errors.

## Evidence and root cause

Investigation used checkout `6aa3871a946` on Linux.
The issue reports Windows x64 release 0.99.1.
The canonical issue contained no comments or image attachments at investigation time.

The reported Claude terminal notice is:

```text
Internal error: You've hit your 5-hour limit · resets 12:50pm (Europe/Berlin)
```

The reported Codex log notice is:

```text
Your workspace is out of credits. Add credits to continue.
```

The reporter receives only JSON-RPC `-32603` / `Internal error` as the Codex terminal error.
No captured Codex ACP frames accompany the report.
The reproduction below supplies the notice as a synthetic ACP message chunk and does not establish that the reporter receives that chunk.

`routingerr/rules.go` recognizes Claude session and weekly limits, but lacks the five-hour and generic usage forms.
The Codex table lacks the explicit workspace-credit phrase.
Unmatched prompt-send and streaming failures reach `phase.poststart.unknown` as `agent_runtime_error`, with fallback and retry disabled.

`SchedulerService.HandlePostStartFailure` returns unhandled for that classification.
The Office subscriber then reaches `Service.HandleAgentFailure`, which increments consecutive failures and can pause the agent at its configured threshold.
The usual degradation and requeue path never runs.

Codex has an additional propagation dependency.
`Adapter.observeCodexProviderEvidence` stores only marked, high-confidence quota messages for the active prompt.
The missing credit signature leaves the message unmarked, so `sendPrompt` cannot attach it to the matching generic error.

Two temporary Go overlays reproduced these gaps without changing tracked source or permanent tests:

| Probe | Observed result |
| --- | --- |
| `TestIssue5222Probe` | Five-hour, generic Claude usage, and Codex credit cases failed in both post-start phases. Each returned an unknown runtime error with no reset hint. |
| Existing Claude session control | Passed. At observation `2026-11-09T09:10:00Z`, Berlin's `12:50pm` produced `2026-11-09T11:50:00Z`. |
| Bare Codex error control | Passed. `Internal error` remained an unclassified runtime error. |
| `TestIssue5222CodexCreditCorrelationProbe` | Failed. Events were an ordinary message chunk followed by an error. The terminal source stayed `acp_prompt` with no credit notice. |

The probe command shape was:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -overlay /tmp/<probe>/overlay.json ./internal/agent/runtime/routingerr ./internal/agentctl/server/adapter/transport/acp -run '^TestIssue5222' -v -count=1)
```

The temporary files were removed after execution.
The command records the experiment, rather than provide an implementation check that remains runnable after cleanup.

A second disposable overlay supplied the candidate rules, hard-quota precedence entries, and five-hour clock-parser gate.
It passed classifier cases in both phases, with and without HTTP 429, plus prose, fence, split-line, and bare-error controls.
Its Codex replay produced `message_chunk:diagnostic` followed by a `codex_acp` terminal diagnostic containing the credit notice.
The existing `TestRoutedTaskless_PostStartFallback_RetryAndDelayedDuplicateAreSafe` also passed with the reported five-hour notice substituted for its quota fixture.
The real subscriber and scheduler degraded Claude as `quota_limited` and launched Codex on the next attempt.

These results establish a reliable catalogue repair and the supported Codex correlation path.
They do not reproduce a live subscription limit, the Windows installation, or a Codex notice available only in process logs.

## Scope

### In scope

- Provider-scoped Claude five-hour, Claude usage-limit, and Codex workspace-credit signatures.
- Five-hour reset inference through the existing zoned clock parser.
- Existing hard-quota precedence over HTTP 429 metadata.
- Codex same-prompt notice correlation, including negative isolation cases.
- Office provider health, next-candidate routing, and unchanged agent-failure counts under recognized quota failures.
- Dynamic hard-policy and shared-binding circuit compatibility.
- A targeted native Windows CI step for the new classifier and ACP regressions.

### Out of scope

- Treating a bare Codex `Internal error` as provider unavailability or quota evidence.
- Parsing uncorrelated stderr, adding transport fields, or changing diagnostic redaction.
- Guessing a renewal date from a generic usage notice's clock or a weekly notice's clock.
- Automatically resuming agents that were already paused before this correction.
- The issue's separate PAT authentication and npm health-probe observations.
- New recovery UI, locale copy, flags, settings, database schema, manual-recovery rules, or provider-selection policy.

## Technical approach

### Catalogue and timing

Add `claude.stderr.five_hour_limit.v1`, `claude.stderr.usage_limit.v1`, and `codex.stderr.out_of_credits.v1` in `routingerr/rules.go`.
Use the existing fence-aware matcher, diagnostic-line anchors, and horizontal whitespace.
Preserve existing quota-rule priority and require each complete diagnostic phrase.
The rules return `CodeQuotaLimited` and `ConfHigh` through existing invariants.
They require no new classification code or feature flag.

Add all three IDs to `hardQuotaRuleOutranksRateStatus` in `routingerr/routingerr.go`.
Other typed statuses retain precedence, and a metadata-only HTTP 429 remains a rate limit.

Extend `Classify`'s clock-only branch to the exact five-hour rule alongside the existing session rule.
For the new rules, derive both dated and clock text hints from only the unfenced diagnostic line that matched the rule, so fenced or unrelated prose cannot supply a deadline.
Reuse `parseResetClockHintAt`, `Input.OccurredAt`, timezone validation, and structured-hint precedence.
At the fixed observation above, the reported notice must produce `2026-11-09T11:50:00Z` with `ResetHintInferred=true`.
Generic usage and weekly notices cannot establish a renewal date from a clock alone.
They retain structured or valid dated hints and otherwise use existing recovery policy.

### Consumer boundaries

The existing Codex message marker and `observeCodexProviderEvidence` consume the new high-confidence rule.
Preserve the current prompt-generation checks, notification drain, exact generic-error predicate, and cancellation behavior.
No adapter production change is expected.
Do not add a generic error rule as a substitute for missing diagnostic evidence.

Keep Office production paths unchanged.
Use the real event subscriber, routing dispatcher, repository, and scheduler in regressions.
Assert provider degradation, the failed attempt's quota code, the stored retry deadline, requeueing, and selection of the next configured healthy provider.
Provider failures handled by routing must preserve the initial consecutive-failure count, including a count one below the pause threshold.
Repeated recognized failures must not trigger automatic pause.

Dynamic tests use `Engine.ApplyFailure`, default hard-error policies, and an injected clock.
Every candidate on the exhausted credential binding must remain ineligible while its circuit is open.
At least one healthy candidate on another binding must remain selectable.
At the reset deadline, prove exclusive probe eligibility and recovery only after a successful probe.

### Windows validation

The `test-windows` job in `.github/workflows/backend-tests.yml` uses explicit package lists and test filters.
Its native cohort does not currently execute these quota regressions.
Add one bounded step in that cohort for the new classifier and in-memory ACP replay tests.
Use the exact native command in the work order, with existing resource limits and read-only job permissions.
Keep other Windows cohorts unchanged.

| Provider and transport | Identity and evidence | Intended behavior | Verification | Unsupported-shape fallback |
| --- | --- | --- | --- | --- |
| Claude ACP terminal error | `claude-acp`, complete five-hour notice | Hard quota and zoned reset | Classifier and real Office event tests | Invalid clock retains quota without a hint |
| Claude ACP usage notice | `claude-acp`, complete usage-limit phrase | Hard quota without clock-only date inference | Classifier and policy tests | Partial or quoted phrases retain existing classification |
| Codex ACP message and terminal error | Current prompt's credit notice, then exact `-32603` / `Internal error` | Existing typed `codex_acp` quota projection | ACP fixture replay and Office typed-error test | Missing notice or mismatched prompt keeps ordinary ACP handling |
| Codex process logs only | No correlated ACP notice | No new classification authority | Generic-error negative regression | Preserve existing handling pending stronger transport evidence |
| Dynamic profile | Current correlated failure, shared binding plus distinct healthy binding | Existing saved hard policy and circuit | Dynamic engine regression | Existing output/effect, ownership, and generation guards |
| Fixed Kanban profile or another provider | Existing consumer identity and policy | Existing recovery policy | Existing classifier/policy checks and provider negatives | No new automatic replay authority |

## Tests

All new names below are planned permanent regressions.
The investigation probes above are disposable evidence only.

| Acceptance criteria | File and regression |
| --- | --- |
| Claude `.9`, `.10` | `routingerr/provider_quota_notices_test.go`: `TestClassifyClaudeFiveHourLimit`, `TestClassifyClaudeFiveHourLimitInfersZonedReset`, `TestClassifyClaudeFiveHourLimitBindsResetToDiagnosticLine`, `TestClassifyClaudeUsageLimit` |
| Claude `.9`, `.11`; Codex `.5`, `.6` | Same file: `TestClassifyCodexWorkspaceCredits`, `TestProviderQuotaNoticeCollisionsAndPrecedence` |
| Codex `.6` | `transport/acp/codex_workspace_credits_test.go`: `TestCodexWorkspaceCreditsProjectsMatchingGenericPromptError`, `TestCodexWorkspaceCreditsPreservesPromptIsolation` |
| Claude `.6`, `.11`; Codex `.7` | `dynamic/engine_provider_quota_test.go`: `TestEngineRoutesProviderQuotaNotices` |
| Claude `.11`; Codex `.7` | `office/service/provider_quota_routing_test.go`: `TestOfficeProviderQuotaPostStartFallback`, `TestOfficeProviderQuotaDoesNotAutoPause` |

Claude numbers refer to `AC-AGENTS-CLAUDE-SESSION-LIMIT-001`.
Codex numbers refer to `AC-AGENTS-CODEX-USAGE-LIMIT-001`.
Existing reset-parser, quota-policy, route-health, stale-event, and taskless fallback tests remain compatibility evidence.

## E2E tests

Backend integration tests provide the end-to-end evidence for this repair.
ACP replay covers the message-to-terminal projection.
Office tests drive real lifecycle events through routing, persisted health, route attempts, and the next scheduler launch.
Cover task-backed and taskless runs, and both configured provider orders.
Use a mixed candidate set containing the exhausted route and a healthy alternative.
The work order also covers repeated failures near the automatic-pause threshold.

No rendered component or interaction changes, so no Playwright scenario or UI preview is needed.
No live account exhaustion, paid run, or main-instance database is required.

## Work orders

- [x] [Task 01: Classify quota notices and prove provider recovery](task-01-classify-quota-notices.md)

## Verification results

Implementation completed on 2026-11-10:

- Added `claude.stderr.five_hour_limit.v1`, `claude.stderr.usage_limit.v1`, and `codex.stderr.out_of_credits.v1` with fence-aware diagnostic-line matching and horizontal whitespace.
- Added all three rules to `hardQuotaRuleOutranksRateStatus`; only the five-hour rule joined the clock-only reset path.
- Scoped text-derived reset hints for the new notice rules to their matching unfenced diagnostic line, including the dated parser. Regressions cover fenced, prose, other-channel, and quoted dated text before a genuine notice, plus a notice without a reset beside unrelated text.
- Added classifier, ACP correlation/isolation, dynamic circuit, and real Office routing regressions, including task-backed and taskless Claude-to-Codex and Codex-to-Claude orders.
- `go test -trimpath -tags fts5 ./internal/agent/runtime/routingerr ./internal/agent/runtime/dynamic ./internal/agentctl/server/adapter/transport/acp ./internal/office/scheduler ./internal/office/service -count=1`: passed.
- `bash scripts/lint-go-changed ...` for the six changed Go files: 0 issues.
- `python3 scripts/list-docs.py validate`: passed, 446 decisions and 1963 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- PR-documentation `validateCoverage` preflight: covered, with no errors.
- `git diff --check`: passed.
- The native Windows quota-regression step was added to the existing `test-windows` native cohort and selects every `TestClassifyClaudeFiveHourLimit*` reset-provenance regression. It was not executed on the Linux implementation host; hosted CI owns that result.
- After the reset-provenance correction, the affected package tests, changed-Go lint, documentation checks, workflow contract/pinning checks, and coverage preflight passed again.

Investigation results on 2026-11-10:

- Original-source disposable classifier and Codex correlation probes failed for the expected missing-signature reasons.
- Disposable candidate probes passed in `routingerr`, ACP transport, and the real Office taskless fallback integration.
- Existing baseline passed: `go test -trimpath -tags fts5 ./internal/agent/runtime/routingerr ./internal/agentctl/server/adapter/transport/acp -run 'TestClassify|TestParse|TestCodexUsageLimit' -count=1`.
- `python3 scripts/list-docs.py validate`: passed, 446 decisions and 1963 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- PR-documentation `validateCoverage` preflight: covered, with no errors.
- `git diff --check -- docs/specs docs/plans`: passed.
- Requirement IDs, acceptance IDs, design mappings, package links, and new-file whitespace audit: passed.
- GitHub assignment: verified issue #5222 is open and assigned to `carlosflorencio`.

The overlay experiment remains feasibility evidence only; the permanent regressions and production changes above are the completion record.

## Companion packages and documentation

The completed [Claude session-limit package](../claude-session-limit-classification/plan.md) owns the existing clock parser and earlier session-limit tests.
The [Codex usage-limit package](../codex-usage-limit-classification/plan.md) records the existing prompt-correlation implementation.
Their original scopes and recorded results remain unchanged.
This package owns only the new notice variants and their consumer regressions.

Public Office routing guidance already describes recognized quota fallback and provider recovery.
This package introduces no new operator action, command, setting, or recovery promise.
The documentation check therefore requires internal specifications and delivery records only.
Existing provider-neutral recovery and provider-policy ADRs already define these boundaries.
No new ADR is required.

## Risks

- The Codex fix requires a complete ACP notice from the same prompt. Process-log-only reports remain unsupported by this bounded repair.
- A clock with no renewal period cannot establish a reset date. Generic usage notices must retain existing backoff.
- New patterns must reject assistant quotations, fences, and line-spanning matches to avoid degrading healthy providers.
- Quota flags do not bypass replay or continuation admission. Keep current safety checks and saved policies.
- Windows behavior was reported, while disposable probes ran on Linux. The targeted native CI step must pass during implementation delivery.
- Already-paused agents remain paused until the existing explicit resume action occurs.
