---
id: "03-explain-errors-and-prove-continuity"
title: "Explain errors and prove continuity"
status: done
wave: 3
depends_on:
  - "01-retain-failed-turn-runtime"
  - "02-reuse-runtime-for-recovery"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-TURN-CONTINUITY-001
  - REQ-PLATFORM-TURN-CONTINUITY-002
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
acceptance_criteria:
  - AC-PLATFORM-TURN-CONTINUITY-001.1
  - AC-PLATFORM-TURN-CONTINUITY-001.2
  - AC-PLATFORM-TURN-CONTINUITY-001.3
  - AC-PLATFORM-TURN-CONTINUITY-001.4
  - AC-PLATFORM-TURN-CONTINUITY-001.5
  - AC-PLATFORM-TURN-CONTINUITY-001.6
  - AC-PLATFORM-TURN-CONTINUITY-001.7
  - AC-PLATFORM-TURN-CONTINUITY-001.8
  - AC-PLATFORM-TURN-CONTINUITY-002.1
  - AC-PLATFORM-TURN-CONTINUITY-002.2
  - AC-PLATFORM-TURN-CONTINUITY-002.3
  - AC-PLATFORM-TURN-CONTINUITY-002.4
  - AC-PLATFORM-TURN-CONTINUITY-002.5
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.11
system_design:
  - ../../specs/platform/system-design/transient-turn-runtime-continuity.md
  - ../../specs/platform/system-design/provider-error-recovery.md
---

# Task 03: Explain errors and prove continuity

## Summary

Show the real provider error without startup recovery controls when ACP remains usable.
Correct the reported legacy historical projection and keep genuine runtime recovery intact.
Prove manual and automatic continuation through desktop and phone Chat using deterministic process traces.

## In scope

- Require explicit bootstrap evidence before choosing the startup historical view model; an execution ID alone is insufficient.
- Render retained failures through the existing inline status row, normal composer, details disclosure, and supported model selector.
- Preserve truthful provider diagnostics, actual attempt counts, and refusal/cancellation/exhaustion distinctions.
- Keep one durable entry across reload, pagination, another viewer, and a later successful turn.
- Share projection rules across Chat surfaces; preserve actual startup, workspace, and terminal runtime recovery controls.
- Localize new product copy in English, Portuguese, Simplified Chinese, Traditional Chinese variants, Japanese, and Korean.
- Generate Traditional Chinese and pseudo catalogs through existing tooling.
- Add a deterministic capacity-after-output/tools mock fixture with a settled RPC and an open ACP process.
- Cover read, write, and unknown-effect distinctions through backend tests; the browser fixture proves visible tool activity and manual follow-up.
- Expose process/connection/native/execution identity and initialize/new/load/resume counts through existing test tracing patterns.
- Exercise follow-up, supported model change, replay, idle cancel, exhaustion, and enabled Cursor continuation without restart.
- Retain true-loss restoration, Data-only conservative recovery, and real startup-control scenarios.
- Update delivered public session documentation and scoped mock-agent guidance where current wording assumes mandatory teardown.

## Out of scope

- New model-selection UI, overlays, retry owners, provider support, metrics dashboards, or restoration controls.
- Mutating the reported live session or weakening genuine runtime-loss recovery to make tests pass.
- Rewriting historical retry counts where persisted evidence cannot establish what happened.

## Acceptance

1. Desktop and phone show the capacity condition in one inline error entry with a usable composer.
   The existing legacy capacity record no longer becomes startup fallback copy; genuine startup records retain their recovery actions.
2. Browser traces prove the exact runtime survives manual follow-up, supported model change, eligible replay, and enabled live-runtime continuation.
   Reload, another viewer, cancelled/exhausted recovery, and later success retain truthful history without duplicate errors.
3. Touch interaction, wrapping, scroll ownership, draft preservation, localization, and true-loss recovery checks pass.
   Public and scoped operational docs describe the delivered behavior, and final work-order results contain actual commands and outcomes.

## ASCII UI preview

Excerpt of [UI-01 and UI-02 in the manifest](plan.md#ascii-ui-preview).
Applicable criteria: AC-PLATFORM-TURN-CONTINUITY-002.1 through 002.5, plus 001.3 and 001.4.

```text
UI-01: Task Chat, capacity failure after tool activity
Before, desktop history
  [Resolved] The agent could not start. Retry resume...

After, desktop
  [!] Selected model is at capacity.
      Try again or choose another model.
      [> Technical details]
  [Type a message...                                 ]
  [6.1 Sol v]                                  [Send]

After, phone
  [!] Selected model is at
      capacity. Try again or
      choose another model.
      [> Technical details]
  [Type a message...            ]
  [6.1 Sol v]              [Send]

UI-02: Task Chat, existing automatic recovery notice
  Temporary provider error. Retry in 5s. [Cancel]
After idle cancellation or exhaustion, runtime usable
  [!] Recovery stopped. You can send another message.
  [Type a message...]
  [Model v]                                  [Send]
```

The transcript retains its scroll owner; the composer remains in its existing fixed safe-area region.
Phone controls use existing touch interactions with wrapping; no new sheet or overlay is added.
Inline hierarchy, provider condition, and normal composer are required.
Exact localized copy, spacing, icons, and model label are illustrative.
Details use existing sanitized disclosure and copy behavior.

## Verification

Read scoped web and mock-agent guidance, `/mobile-parity`, `/e2e`, `/tdd`, and `/docs-maintainer` before implementation.
Use a Red-Green-Refactor fixture/model test before each changed behavior.
In a fresh worktree, install workspace dependencies before any pnpm command.
Run this block from the repository root:

```bash
if [ ! -d apps/node_modules ]; then (cd apps && pnpm install --frozen-lockfile); fi
(cd apps/backend && go test -trimpath -race -tags fts5 ./cmd/mock-agent -count=1)
(cd apps/web && pnpm exec vitest run components/task/chat/messages/action-message-recovery.test.tsx components/task/chat/messages/action-message.test.tsx components/task/chat/session-recovery-model.test.ts components/task/chat/session-bootstrap-recovery-model.test.ts lib/session-last-agent-error.test.ts lib/session-recovery-presentation.test.ts)
(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:pseudo && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/chat/messages/action-message-recovery-history.tsx components/task/chat/messages/action-message.tsx components/task/chat/session-bootstrap-recovery-model.ts lib/session-recovery-presentation.ts)
(cd apps/web && pnpm e2e:run --project chromium e2e/tests/session/transient-turn-runtime-continuity.spec.ts e2e/tests/session/transient-retry.spec.ts e2e/tests/session/provider-interruption-continuation.spec.ts e2e/tests/session/transient-retry-transport-lost.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/session/mobile-transient-turn-runtime-continuity.spec.ts e2e/tests/session/mobile-transient-retry.spec.ts e2e/tests/session/mobile-provider-interruption-continuation.spec.ts e2e/tests/session/mobile-session-error-recovery-ui.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
git status --short -- docs/plans/transient-turn-runtime-continuity
```

The two new Playwright files must be created before running the documented browser commands.
Run desktop and phone commands sequentially; retain the managed runner's worker and memory limits.
Compare the rendered affected regions with UI-01 and UI-02, including expanded details.
Assert no required Resume/Start fresh controls on retained failures, plus normal send and model-selector behavior.
Use public interaction to submit; use test traces only to prove process identity and initialization/restoration counts.
Do not infer continuity from a boot-row count or task-session identity alone.
Update existing replay/continuation expectations for retained cases while keeping actual-loss scenarios.
Run changed suites and extend the exact lint/test commands if implementation touches additional model files.
After product checks, rerun work orders 01 and 02 and record each result in its owning file.
Run the exact [documentation coverage preflight](plan.md#documentation-coverage-preflight) against actual changes before completing the implementation package.

## Files likely touched

- `apps/web/components/task/chat/messages/action-message-recovery-history.tsx`, `action-message.tsx`, and their existing recovery/message tests.
- `apps/web/components/task/chat/session-bootstrap-recovery-model.ts`, `session-recovery-model.ts`, and adjacent tests where required.
- `apps/web/lib/session-last-agent-error.ts`, `session-recovery-presentation.ts`, and adjacent tests where required.
- `apps/web/src/locales/*/task.json` and other owning namespaces; generated Traditional Chinese and pseudo output.
- `apps/backend/cmd/mock-agent/main.go`, `handler.go`, capacity fixture source/tests, and existing tracing helpers.
- `apps/backend/cmd/mock-agent/interruption_continuation.go`, existing overload/continuation tests, and `AGENTS.md`.
- New desktop `apps/web/e2e/tests/session/transient-turn-runtime-continuity.spec.ts` and phone `mobile-transient-turn-runtime-continuity.spec.ts`.
- Existing transient-retry, provider-interruption-continuation, transport-loss, and mobile session-error recovery suites named in verification.
- `apps/web/e2e/helpers/provider-interruption-continuation.ts`, `transient-retry.ts`, `session.ts`, `causal-waits.ts`, and `api-client.ts` as needed.
- `docs/public/sessions-and-review.md`; `docs/public/agent-communication.md` only if its current recovery explanation is affected.
- This package's work-order results and manifest verification results.

## Dependencies

[Task 01](task-01-retain-failed-turn-runtime.md) and [Task 02](task-02-reuse-runtime-for-recovery.md) must pass first.
The browser fixture verifies their integrated outcomes instead of introducing another backend recovery path.

## Risks

- Legacy records may lack explicit startup fields. Preserve provider explanations without removing controls for proven bootstrap failures.
- Backend error text and localized UI labels have different ownership; avoid translating semantic classifier comparisons.
- Persisted history must not become a source of runtime reuse authority after reload or backend restart.
- Mock tracing must prove physical reuse without relying on unstable production timing.
- Generators can alter unrelated locale keys. Inspect the diff and retain only intended catalog updates.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/transient-turn-runtime-continuity.md): all runtime and Chat criteria.
- [Design](../../specs/platform/system-design/transient-turn-runtime-continuity.md): durable projection, desktop/phone, and verification.
- [Decision](../../decisions/2026-10-03-transient-turn-runtime-lifetime.md).
- [Manifest UI previews and scenario matrix](plan.md#ascii-ui-preview).
- Existing `action-message-recovery.test.tsx`, mobile Chat layout, transient notices, mock process traces, and continuation E2E helpers.

## Results

Completed in the primary session. Retained provider failures render as truthful turn errors, preserve the normal composer, and keep retry refusal, cancellation, and exhaustion feedback tied to actual attempts. The mock trace proves the ACP process and connection survive capacity failure, follow-up, model change, retry settlement, and continuation.

- `(cd apps/web && pnpm exec vitest run components/task/chat/message-renderer.test.tsx components/task/chat/messages/action-message-recovery.test.tsx components/task/chat/messages/action-message.test.tsx components/task/chat/session-recovery-model.test.ts components/task/chat/session-bootstrap-recovery-model.test.ts lib/session-last-agent-error.test.ts lib/session-recovery-presentation.test.ts)`: 7 files and 117 tests passed, including retained error rendering, legacy history, recovery details, and recovery-model cases.
- `(cd apps/web && pnpm run typecheck)`: passed.
- ESLint with `--max-warnings 0` and Prettier checks passed for changed frontend and E2E files.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet`: passed. Locale catalogs include all required languages; unrelated generated labels were excluded from the diff.
- Final desktop managed Playwright run: 23 tests passed across capacity continuity, transient replay, provider continuation, and real transport loss. The full suite was rerun after correcting the mock capacity prompt fixture and after wiring production lifecycle acknowledgement.
- Final phone managed Playwright run: 12 tests passed across capacity continuity, transient replay, provider continuation, and startup recovery.
- Exact managed Playwright commands:

  ```bash
  (cd apps/web && pnpm e2e:run --host --shards 1 --project chromium --retries=0 e2e/tests/session/transient-turn-runtime-continuity.spec.ts e2e/tests/session/transient-retry.spec.ts e2e/tests/session/provider-interruption-continuation.spec.ts e2e/tests/session/transient-retry-transport-lost.spec.ts)
  (cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome --retries=0 e2e/tests/session/mobile-transient-turn-runtime-continuity.spec.ts e2e/tests/session/mobile-transient-retry.spec.ts e2e/tests/session/mobile-provider-interruption-continuation.spec.ts e2e/tests/session/mobile-session-error-recovery-ui.spec.ts)
  ```
- Backend build and frontend pseudo-locale QA build passed during implementation.
- Public documentation validation, documentation catalog/specification checks, the actual-change work-order coverage preflight, and `git diff --check` are recorded in the completed manifest.
