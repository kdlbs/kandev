---
id: "02-explicit-resume-recovery"
title: "Explicit recovery and visible settings omission"
status: done
wave: 2
depends_on: ["01-strict-auggie-startup"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-EXPLICIT-RESUME-SETTINGS-001
acceptance_criteria:
  - AC-AGENTS-EXPLICIT-RESUME-SETTINGS-001.2
  - AC-AGENTS-EXPLICIT-RESUME-SETTINGS-001.3
  - AC-AGENTS-EXPLICIT-RESUME-SETTINGS-001.4
  - AC-AGENTS-EXPLICIT-RESUME-SETTINGS-001.5
  - AC-AGENTS-EXPLICIT-RESUME-SETTINGS-001.6
  - AC-AGENTS-EXPLICIT-RESUME-SETTINGS-001.7
system_design:
  - ../../specs/agents/system-design/explicit-resume-settings.md
---

# Task 02: Explicit recovery and visible settings omission

## Summary

Deliver the failed-session Resume exception end to end, keeping the conversation
and saved configuration while omitting mode/model startup overrides for one attempt.
Explain the action before dispatch and record successful recovery visibly.

## In scope

- Extend the existing recovery request with the optional typed settings policy;
  validate failed Auggie eligibility and token under existing admission guards.
  Propagate through launch/executor/lifecycle without an ambient or persisted flag.
- Omit profile, runtime, workflow, generic config and provider creation carriers
  for both mode and model; retain unrelated known settings. Do not mark ready
  early. Preserve the same provider token and forbid implicit session/new fallback.
- Wire shared recovery helpers to every existing failed-session Resume surface.
  Ordinary pause/resume and automatic hooks omit the exception. Later explicit
  selectors work normally; later independent launches enforce saved choices.
- Persist a structured attempt-owned notice and render truthful provider values.
  Preserve unknowns; test failed notice writes, replay and delayed stale success.
- Add localized disclosure and success copy in all catalogs, generated Traditional
  Chinese translations, and desktop/mobile E2E with outbound mock ACP evidence.
- Use docs-maintainer during implementation to update `docs/public/sessions-and-review.md`
  and relevant Auggie profile-policy guidance in `docs/public/agents-and-profiles.md`.

## Out of scope

Changing shared ACP confirmation, silently resetting the provider conversation,
profile persistence changes, other-provider/Office rollout, new recovery dialogs,
and combining this action with branch replacement or managed-runtime recovery.

## Acceptance

- Only an eligible explicit recovery request skips every mode/model carrier;
  same identity, authorization, cancellation and state ownership are preserved.
- A recovered session accepts the next prompt and later explicit selections;
  saved values survive and the exception does not leak to another attempt.
- Disclosure, pending state, errors, effective selectors and durable notice work
  on desktop and phone, including reload and repeated attempts.

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

Full preview and test matrix: [plan.md](plan.md#ascii-ui-preview).

## Verification

Implement the named Task 02 tests in the plan with TDD. In a fresh worktree,
first run `(cd apps && pnpm install --frozen-lockfile)`. From repository root:

```bash
(cd apps/backend && go test -tags fts5 ./internal/orchestrator/... ./internal/agent/runtime/lifecycle ./internal/agentctl/server/adapter/transport/acp)
(cd apps/web && pnpm exec vitest run lib/services/session-recovery-service.test.ts components/task/chat/session-bootstrap-recovery-card.test.tsx components/task/chat/session-stopped-banner.test.tsx components/task/simple/components/run-error-entry.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --host --project chromium tests/session/session-resume-settings-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/session/mobile-session-resume-settings-recovery.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run managed E2E commands sequentially; they build their tested artifacts. Add any
new helper test file to this block before completing implementation. Record real
Auggie smoke availability separately; mock routing tests do not prove provider behavior.

## Files likely touched

- `apps/backend/internal/orchestrator/handlers/handlers.go` and handler tests
- `apps/backend/internal/orchestrator/session_launch.go` and `session_launch_test.go`
- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/agent/runtime/lifecycle/session.go` and `session_test.go`
- Existing runtime launch request types and session status-message persistence path
- `apps/web/lib/services/session-recovery-service.ts` and its tests
- `apps/web/hooks/domains/session/use-session-recovery-actions.ts`
- `apps/web/components/task/chat/session-bootstrap-recovery-content.tsx`, recovery model/card, stopped banner and tests
- `apps/web/components/task/simple/components/run-error-entry.tsx` and tests
- `apps/web/src/locales/` catalogs
- New desktop/mobile specs listed in the plan and mock-agent fixture support
- Public documentation named above

## Dependencies

Task 01. Retain strict baseline tests as negative coverage throughout this task.

## Risks

A stale success event or an omitted generic-config carrier can undermine the
attempt boundary. Use deterministic delayed-response tests and outbound ACP
request assertions. Provider restore itself may still reject an obsolete model.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/explicit-resume-settings.md)
- [Design](../../specs/agents/system-design/explicit-resume-settings.md)
- Existing `session_launch_test.go`, `session-resume-recovery.spec.ts`, and
  `mobile-session-resume-recovery.spec.ts` patterns.

## Results

Implementation is in place across explicit admission, executor/backendapp/
lifecycle policy transport, startup-carrier omission, persisted-versus-effective
selector state, durable success notice, recovery eligibility/disclosure, and
desktop/mobile recovery surfaces. Provider-restored mode/model carriers are
omitted for one accepted failed Auggie ACP task Resume; lifecycle keeps the same
native load token and accepts a later prompt. Saved profile/runtime selections
remain unchanged, later selector operations persist, and ordinary attempts keep
strict behavior. A test-only opt-in Auggie mock uses the existing mock ACP
process and records load identity and selector requests for the browser specs.
Public session and profile guidance plus all six locale catalogs were updated.

Red-green evidence covers startup carrier omission, eligibility/no-launch
rejections, typed policy transport, effective-versus-saved selector state,
durable notice ownership, and the shared recovery renderer. Lifecycle outbound
tests caught model/mode requests that escaped the recovery policy. A provider
load-order regression caught an effective model report deferred behind an
empty synthetic snapshot. The shared-card regression found duplicate
read-only status text; semantic summary visibility now removes the duplicate
while preserving the announcement and distinct error/branch/guard guidance.

The reported live admission failure came from comparing the agent's database
UUID with the `auggie` provider slug and dropping `NativeSessionResume` in the
backendapp adapter. Both mappings are fixed and covered by UUID-shaped admission
and real adapter tests. The user's later retry resumed the same session and
native token, reached ready at 19:27:58 +0100, and reported `gemini-3-7-flash`
with eight choices. The provider-restored startup guard now publishes that
report while preserving strict-startup deferral. No real Auggie smoke test was
run. The explicit Default-mode tooltip was retained because the provider did
not confirm the user's later selection. Successful boot persists
`recovery_resolved_at`, and recovery ownership clears from that durable state.

Verification:

- The live-failure regressions passed after the fix:
  `go test -tags fts5 ./internal/orchestrator -run '^(TestValidateProviderRestoredRecoveryEligibility|TestRecoverSessionProviderRestoredPolicyReachesLifecycleLaunchRequest|TestRecoverSessionProviderRestoredRejectsIneligibleSessionsWithoutLaunch|TestResumeTaskSessionWithOptionsRechecksProviderRestoredEligibilityUnderAttempt)$' -count=1`
  and `go test -tags fts5 ./internal/backendapp -run '^(TestLifecycleAdapter_ResolveAgentProfileForwardsNativeSessionResume|TestBuildLifecycleLaunchRequestCarriesSessionSettingsPolicy)$' -count=1`.
- The adjusted `go test -tags fts5 ./internal/orchestrator/...` subtree passed
  (all eight packages). Focused admission, stale-attempt, lifecycle settings,
  notice, snapshot, and handler tests passed.
- The full affected Go package run passed `internal/agent/runtime/agentctl`,
  `internal/agentctl/server/api`, `internal/agentctl/server/adapter/transport/acp`,
  `internal/orchestrator/watcher`, `cmd/mock-agent`, and
  `internal/agent/registry`. `TestBuildLifecycleLaunchRequestCarriesSessionSettingsPolicy`
  passed separately.
- The full merged backendapp suite passed. The full lifecycle package passed
  with the four unchanged-baseline SSH orphan-process tests documented in Task
  01 skipped; focused changed lifecycle tests passed. Other affected backend
  packages passed, and the Linux backend build and Vite/fixture builds passed.
- Affected frontend recovery, selector, state/hydration, and renderer suites
  passed (15 files, 186 tests). The focused recovery renderer suites passed
  (3 files, 50 tests). Web typecheck, i18n checks, Vite build, spec validation,
  spec lint, and harness lint passed. The desktop/mobile E2E helper and both
  specs pass focused ESLint and Prettier checks.
- `session-resume-settings-recovery.spec.ts` (Chromium) passed 1/1 in 1.5m;
  capture run log: `/private/tmp/kandev-pr-final-desktop-capture.log`.
  `mobile-session-resume-settings-recovery.spec.ts` (mobile-chrome) passed 1/1
  in 1.4m; capture run log: `/private/tmp/kandev-pr-final-mobile-capture.log`.
  Managed Docker desktop and mobile E2E each passed (one test per viewport).
  Both confirmed the same native ACP session, strict settings failure before
  recovery, provider-restored retry without model/mode overrides before the next
  prompt, later model and mode selection, and the durable success notice after
  reload. The capture helper waits for the provider-confirmed mode and places
  the notice and selectors in the viewport. All six publication captures were
  visually inspected, validated, and compressed; the final manifest is
  `apps/web/.pr-assets/manifest.json`.
- `python3 scripts/list-docs.py validate`,
  `python3 scripts/lint-spec-files.py --all`, and `git diff --check` passed.

Post-merge review regressions passed on 2026-09-30. Mode and model event
consumers share a per-session serialization boundary across stale checks,
fresh snapshot reads, merges, persistence, and broadcast. The ACP adapter keeps
settings generations monotonic across LoadSession and ResetSession; a new
attempt replaces the effective snapshot, including on a strict mode-only
event with an empty legacy attempt ID. Red-green coverage verifies concurrent
mode/model updates preserve both values and generations, older reports do not
broadcast, and old-session notifications remain rejected after reset and load.
The concurrency regression passed under `-race -count=3`.

Earlier post-merge package and E2E receipts above predate the source-epoch
remediation. The final checks passed on 2026-09-30: orchestrator 59.571s,
executor 26.656s, backendapp 52.319s, and ACP transport 12.513s. The executor
aggregate excluded only `TestMissingCheckoutRecoveryLaunchAndResume`; its three
`/dev/fd/3` subcases are established unchanged-baseline environment failures.
The final lifecycle package passed in 145.860s with the four baseline SSH
orphan-process tests and `TestMissingCheckoutRecoveryLifecycleRestoresAndProjectsSelectedWorkspace`
excluded; the latter's `/dev/fd/3` permission failure also reproduces on
unchanged main. The focused source-reservation, attempt provenance, and
projection regressions passed under `-race -count=3`. Scoped golangci-lint
reported 0 issues; gofmt, `git diff --check`, documentation validation, and
spec lint passed.

Final managed Docker runs rebuilt the Linux backend, Vite bundle, and fixture
plugin. Chromium passed 1/1 in 1.4m; mobile-chrome passed 1/1 in 1.3m using the
same artifacts. Both preserved the same native ACP token, omitted model/mode
overrides for recovery, accepted a later prompt and selectors, and displayed
the durable notice after reload. The six validated publication PNGs and
manifest were restored from the immutable media commit after Playwright
cleanup. No UI source changed in this review remediation.

This work order is `done`. The lifecycle package excluded four SSH
orphan-process tests and one `/dev/fd/3` missing-checkout test, all reproduced
on the unchanged baseline. Session-less recovery retains its legacy zero source
generation and strict report projection; durable epochs are reserved only for
task-session-backed adoption. No real Auggie smoke test was run. The published
head's frontend CI failure was limited to four recovery-hook test cases missing
a `StateProvider`; the test fixture is fixed and the updated 17-file, 194-test
web suite passed locally. CI and review for the next pushed head remain external
delivery checks.
