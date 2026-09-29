---
created: 2026-09-29
status: done
requirements:
  - REQ-AGENTS-EXPLICIT-RESUME-SETTINGS-001
system_design:
  - ../../specs/agents/system-design/explicit-resume-settings.md
legacy_specs: []
---

# Fix Plan: Explicit Auggie Resume Without Mode or Model Overrides

## Overview

Implement the user's strict-first, explicit-recovery behavior. Auggie task start
and ordinary/automatic resume fail if the effective selected model or mode cannot
be applied. Clicking Resume on a failed session retries the existing conversation
without either override. The saved profile and session selections stay intact.

[Requirements](../../specs/agents/requirements/explicit-resume-settings.md),
[design](../../specs/agents/system-design/explicit-resume-settings.md), and
[decision](../../decisions/2026-09-29-explicit-resume-settings.md) own the contract.
This replaces the earlier blanket fallback and RPC-confirmation proposals.

## Evidence and settled scope

The reported task failed at 2026-09-29 09:44:00 +0100 because `default` was
unconfirmed, before the first prompt. The resume token was retained. Logs do not
prove Auggie's exact ACP response behavior, and this fix does not need to assume it.

In scope: Auggie task startup/resume policy, explicit recovery transport and
attempt ownership, omitted startup layers, preserved identity, truthful notices,
and later model/mode selection. Other providers and Office retain their current
policy. No shared-settings change, automatic retry fallback, new profile toggle,
or ACP confirmation relaxation. Fresh start is not this recovery path.

## Technical approach

1. Establish strict Auggie task startup/resume enforcement without changing the
   persisted profile policy or other model-policy consumers.
2. Deliver explicit recovery end to end: optional `settings_policy:
   provider_restored` on `session.recover`, server eligibility checks, typed
   attempt propagation, omitted profile/runtime/workflow model and mode carriers,
   and recovery UI disclosure plus a durable success notice.

Use `Service.RecoverSession`, `LaunchSessionRequest`, `executor.ResumeOptions`,
and `SessionManager.InitializeAndPromptWithLayers`. Do not infer authorization
from `StartModelPolicy.AutoFallback`, resume tokens, or generic resume intent.
Keep `applyExplicitSessionMode` and ACP confirmation unchanged for real requests.

| Provider/path | Expected behavior | Evidence/fallback |
| --- | --- | --- |
| Auggie ACP task start/ordinary resume | Exact selected settings or failure | Task/lifecycle integration; no implicit alternative |
| Auggie failed-session recovery Resume | Same identity; omit both overrides | Handler-to-adapter assertions and desktop/mobile E2E |
| Auggie later user selection | Normal selection/confirmation | Selector tests; refused selection stays an error |
| Other providers/Office | Existing profile policy | Negative scope regression tests |
| Unknown config-option identity | Do not guess mode/model mapping | Omit ambiguous optional startup option with notice |
| Missing provider token | Recovery error | Never silently create a new conversation |

## ASCII UI preview

UI-01: Failed Auggie session, existing recovery card. Proposed copy is illustrative;
explanation before action, shared pending state, and inline errors are required.

```text
Desktop
Session startup needs attention
The selected mode/model could not be applied.
Resume keeps this conversation and skips mode/model overrides for this attempt.
[Resume session] [Restore read-only workspace] [Start fresh session]
[Technical details v]

Phone (same chat scroll owner)
Session startup needs attention
The selected mode/model could not be applied.
Resume keeps this conversation and skips
mode/model overrides for this attempt.
[           Resume session            ]
[     Restore read-only workspace      ]
[        Start fresh session          ]
[Technical details v]

Pending: [Resuming...] and equivalent actions disabled.
Failure: inline cause remains; Resume becomes available again.
Success: "Session resumed without mode/model overrides."
         Provider-reported selectors and composer remain available.
```

UI-01 maps to AC-AGENTS-EXPLICIT-RESUME-SETTINGS-001.2, .5, and .7.
No new modal or scroll region. Phone/coarse-pointer targets are at least 44px;
fine-pointer desktop controls retain 28px. Existing mobile selectors use
`MobilePickerSheet`. Success notice remains in history on reload.

## Tests

Proposed test names (add to the named existing suites):

| Criteria | Suite and evidence |
| --- | --- |
| .1 | `lifecycle/session_test.go`: `TestAuggieTaskStartAndResumeRequireSelectedSettings`, table-driven for missing/rejected model, refused/clamped/unconfirmed mode, empty selection, and fallback settings |
| .1, .6 | `orchestrator/session_launch_test.go`: `TestAuggieStrictPolicyIsTaskScoped`, asserting start and ordinary/automatic resume and unchanged Office/other-provider policy |
| .2-.4, .6 | `orchestrator/session_launch_test.go`: `TestExplicitResumeSettingsAdmission`, covering identity, ownership, policy, wrong action/provider/state, legacy request, cancellation and duplicate/stale attempts |
| .2-.4 | `lifecycle/session_test.go`: `TestExplicitResumeSkipsAllModeModelCarriers`, asserting no model/mode RPC or equivalent config/creation override, same load/resume token, readiness and next prompt |
| .4 | Same suite: `TestRecoveredSessionAllowsSelectionAndNextLaunchIsStrict`, including saved profile/session readback |
| .5-.6 | `orchestrator/session_launch_test.go`: `TestExplicitResumeNoticeAttemptOwnership`, covering one persisted notice, replay, failed write/retry, and stale completion |
| .2, .5, .7 | Recovery service and bootstrap-card web tests: request policy, disclosure, pending/error/success, unknown values and noneligible sessions |

Existing ACP no-invented-value, clamp, cancellation, and late-report tests remain
regressions, not tests to weaken. Mock-provider request capture is mandatory;
a real Auggie smoke check is supplementary and uses only a disposable session.

## E2E tests

Add `tests/session/session-resume-settings-recovery.spec.ts` (chromium) and
`tests/session/mobile-session-resume-settings-recovery.spec.ts` (mobile-chrome).
Cover strict failure -> disclosed Resume -> same conversation with no overrides
-> next prompt -> explicit model/mode change, persisted notice on reload, and
an unrelated provider failure that still fails. Check duplicate clicks, keyboard
and touch, unknown effective values, and horizontal containment (.1-.7).

## Work orders

- [x] [Task 01: Strict Auggie task startup and resume](task-01-strict-auggie-startup.md) (four baseline-reproduced SSH orphan-process tests remain skipped)
- [x] [Task 02: Explicit recovery and visible settings omission](task-02-explicit-resume-recovery.md) (desktop/mobile E2E and six screenshot captures passed)

Task 02 depends on Task 01. Both work orders were implemented sequentially with
TDD. The Task 01 lifecycle run excludes four failures reproduced unchanged on
the baseline; no related tests were altered.

## Verification results

Implementation and required local checks completed on 2026-09-29. Current
verification receipts:

- The full orchestrator subtree and backendapp suite passed after merge
  restoration. Affected runtime-agentctl, agentctl API, ACP transport, watcher,
  mock-agent, agent registry, and other backend package checks passed. The
  resolved profile UUID and native-session capability regressions passed their
  focused tests.
- The lifecycle package passed with four unchanged-baseline SSH orphan-process
  tests skipped. The exact tests are
  `TestSSHOrphanStopCommandKillsProcessAndDirectChildProcessGroup`,
  `TestSSHOrphanStopCommandSessionDirSurvivesWhenLiveAgentctlMatchesTaskDir`,
  `TestSSHOrphanStopCommandKillsMultipleChildProcessGroupsUnderZsh`, and
  `TestSSHOrphanStopCommandMismatchedIdentityLeavesProcessAlive`. Each fails on
  the unchanged baseline as well. The lifecycle run used `TMPDIR=/private/tmp`;
  the task's changed lifecycle tests passed.
- Affected web recovery, selector, state, and renderer suites passed (15 files,
  186 tests). Web typecheck, i18n checks, Vite build, spec validation, spec lint,
  and harness lint passed. The current E2E helper's focused ESLint and Prettier
  checks also pass.
- Managed Docker desktop and mobile E2E each passed (one test per viewport).
  The scenarios cover strict failure, explicit recovery, rejection and retry,
  same-token next prompt, omitted mode/model startup overrides, later selectors,
  and the durable notice after reload. All six publication captures were
  visually inspected, validated, and compressed; the final manifest is
  `apps/web/.pr-assets/manifest.json`.
- `python3 scripts/list-docs.py validate`,
  `python3 scripts/lint-spec-files.py --all`, and `git diff --check` passed.
  No real Auggie smoke test was run.

Local implementation and verification are complete. PR CI and review remain
external delivery checks.

## Risks

- Provider-restored settings may be more permissive than the requested mode;
  disclose omission before the explicit recovery action.
- A recovered provider conversation may retain an unusable model internally;
  skipping Kandev overrides cannot guarantee success.
- Generic config options and delayed startup layers can reapply rejected values;
  assert actual outbound requests across the complete recovery path.
- Keep strictness scoped to Auggie tasks; model fallback elsewhere is a separate
  existing contract and must not be changed incidentally.
