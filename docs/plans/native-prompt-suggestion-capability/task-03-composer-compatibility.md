---
id: "03-composer-compatibility"
title: "Verify composer compatibility and document limits"
status: done
wave: 3
depends_on:
  - "02-session-continuity"
plan: "plan.md"
requirements:
  - REQ-AGENTS-NATIVE-SUGGEST-001
  - REQ-AGENTS-NATIVE-SUGGEST-002
  - REQ-AGENTS-NATIVE-SUGGEST-003
  - REQ-UI-PROMPT-SUGGEST-001
  - REQ-UI-PROMPT-SUGGEST-002
  - REQ-UI-PROMPT-SUGGEST-003
  - REQ-UI-PROMPT-SUGGEST-004
acceptance_criteria:
  - AC-AGENTS-NATIVE-SUGGEST-001.6
  - AC-AGENTS-NATIVE-SUGGEST-002.5
  - AC-AGENTS-NATIVE-SUGGEST-002.6
  - AC-AGENTS-NATIVE-SUGGEST-003.2
  - AC-AGENTS-NATIVE-SUGGEST-003.5
  - AC-UI-PROMPT-SUGGEST-001.1
  - AC-UI-PROMPT-SUGGEST-001.2
  - AC-UI-PROMPT-SUGGEST-001.3
  - AC-UI-PROMPT-SUGGEST-001.4
  - AC-UI-PROMPT-SUGGEST-002.1
  - AC-UI-PROMPT-SUGGEST-002.2
  - AC-UI-PROMPT-SUGGEST-003.1
  - AC-UI-PROMPT-SUGGEST-003.2
  - AC-UI-PROMPT-SUGGEST-003.3
  - AC-UI-PROMPT-SUGGEST-004.1
  - AC-UI-PROMPT-SUGGEST-004.2
  - AC-UI-PROMPT-SUGGEST-004.3
  - AC-UI-PROMPT-SUGGEST-004.4
  - AC-UI-PROMPT-SUGGEST-004.5
  - AC-UI-PROMPT-SUGGEST-004.6
  - AC-UI-PROMPT-SUGGEST-004.7
  - AC-UI-PROMPT-SUGGEST-004.8
  - AC-UI-PROMPT-SUGGEST-004.9
  - AC-UI-PROMPT-SUGGEST-004.10
system_design:
  - ../../specs/agents/system-design/native-prompt-suggestion-capability.md
  - ../../specs/ui/system-design/composer-prompt-suggestions.md
---

# Task 03: Verify composer compatibility and document limits

## Summary

Prove that new support evidence cannot bypass the existing effective-session gate.
Reuse the desktop and phone suggestion suite. Document the verified compatibility
exception and provider adoption requirement without changing composer behavior.

## In scope

- Extend capability handler tests for raw true with effective false, effective
  loss, older payloads without raw support, and new explicit raw false.
- Reuse helper/hook coverage for stale turns, preference changes, native silence,
  active/queued work, clarification, recovery, dismissal, and empty editable drafts.
  Add focused cases only where an assertion is absent.
- Reuse TipTap and hint tests for literal angle brackets, entities, line breaks,
  exact submit content, and configured shortcuts.
- Change the existing unsupported E2E fixture to
  `KANDEV_MOCK_AGENT_PROMPT_SUGGESTIONS=false` while queueing remains enabled.
  Keep backend restart and user-preference cleanup.
- Extend an existing typing E2E flow with a causal request observer.
  Assert no `message.add` WebSocket request or utility execution while the user types.
  Separate legitimate sends from the bounded observation interval.
- Run existing desktop task/Quick Chat, settings, and phone touch tests.
- Update the public explanation at `docs/public/developer-tools.md`.
  Clarify that native support requires adapter evidence; identify current limits.
  Describe the new declaration as a new opt-in extension requiring bridge adoption.
- Update the native-suggestion section of agentctl `AGENTS.md` to reflect the
  final support/effective contract and point to the Agents design.
- Synchronize this package's results and paired spec lifecycle after all work orders pass.

## Out of scope

New UI markup, interaction, layout, translations, standalone E2E suites,
provider branches in the composer, broad audits, and upstream publication.

## Acceptance

1. Raw support alone never marks a session native. Existing effective payloads,
   including old payloads without raw support, retain their semantics.
   Capability loss hides stored text and retains normal composer/recovery rules.
2. Existing desktop, Quick Chat, and phone flows pass. The unsupported queue-capable
   fixture keeps the ordinary composer. Literal text remains exact and typing
   causes no generation request.
3. Public and scoped docs describe the shipped evidence limits accurately.
   Requirements/designs, the plan, work orders, and command results agree.
   New upstream adoption is not claimed from mock evidence.

## Verification

Install dependencies once from repository root before the first package command.
Skip the install only when this worktree already completed it.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/prompt-suggestion.test.ts hooks/use-prompt-suggestion.test.ts lib/ws/handlers/prompt-suggestions.test.ts lib/ws/handlers/agent-capabilities-prompt-suggestions.test.ts components/task/chat/tiptap-prompt-suggestion.test.ts components/task/chat/composer-prompt-suggestion-hint.test.tsx components/task/chat/use-prompt-suggestion-escape.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint lib/types/session-runtime-payloads.ts lib/ws/handlers/agent-capabilities-prompt-suggestions.test.ts e2e/tests/chat/prompt-suggestions.spec.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/prompt-suggestions.spec.ts tests/settings/prompt-suggestion-settings.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-prompt-suggestions.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short
```

The managed E2E runner rebuilds current backend, mock-agent, and web artifacts.
Run desktop and phone sequentially. Do not use stale `--no-build` artifacts,
additional workers, overlapping full suites, or a developer's instance.

Run the [documentation preflight](plan.md#documentation-validation-commands) on
actual changed paths and linked documents. Inspect discovery counts and retain
each command's actual result. Do not substitute historical pass counts.

This is data normalization within the existing composer. Existing phone
`mobile-prompt-suggestions.spec.ts` supplies mobile parity proof.
There is no new rendered design or ASCII preview.

## Files likely touched

- `apps/web/lib/ws/handlers/agent-capabilities-prompt-suggestions.test.ts`
- `apps/web/lib/prompt-suggestion.test.ts` and `hooks/use-prompt-suggestion.test.ts` (only missing cases)
- `apps/web/e2e/tests/chat/prompt-suggestions.spec.ts`
- Existing helper files only when shared setup needs a small correction
- `docs/public/developer-tools.md`
- `apps/backend/internal/agentctl/AGENTS.md`
- This plan/work-order package and linked Agents/UI specs

The existing TipTap, hint, mobile, and settings suites are validation inputs.
Do not rewrite their tests to mirror the implementation.

## Dependencies

Tasks 01 and 02. Their backend behavior and mock feature advertisement must exist first.

## Risks

A mock that disables its entire vendor namespace would miss the original defect.
A raw support value in source-selection logic would bypass user preference.
Leaked settings or restart overrides would contaminate neighboring E2E tests.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/native-prompt-suggestion-capability.md)
- [Agents design](../../specs/agents/system-design/native-prompt-suggestion-capability.md)
- [UI requirements](../../specs/ui/requirements/composer-prompt-suggestions.md)
- [UI design](../../specs/ui/system-design/composer-prompt-suggestions.md)
- `apps/web/AGENTS.md` and agentctl scoped guidance
- TDD, E2E, mobile-parity, and docs-maintainer skills

## Results

Implemented and verified. The capability handler keeps effective enablement as
the only composer gate and adds regression cases for raw support alone,
effective loss, older payloads without raw support, and explicit denial. The
unsupported E2E fixture now denies suggestion support while prompt queueing
stays advertised, types literal text, asserts no `message.add` or utility
request during the typing interval, and retains the whole-observation
`utilityCalls.length === 0` assertion so an automatic post-turn utility request
is still caught. Public and scoped documentation describe the verified bridge
tuple and the new opt-in extension.

Verification on 2026-10-28 from repository root:

- Focused Vitest block passed: 7 files, 64 tests.
- `pnpm run typecheck` passed (with `NODE_OPTIONS=--max-old-space-size=8192`
  for the local heap).
- Focused ESLint on the changed frontend files reported no issues.
- Chromium E2E passed: 7 tests across `prompt-suggestions.spec.ts` and
  `prompt-suggestion-settings.spec.ts`.
- Mobile Chrome E2E passed: the 44px Use reply touch target fills the draft.
- `node --test scripts/validate-public-docs.test.mjs`: 63 passed.
- `node scripts/validate-public-docs.mjs`: 54 published pages validated.
- `python3 scripts/list-docs.py validate`: 414 decisions, 1,800 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check` passed.

The first local E2E launch stopped in global setup because the prebuilt backend
artifacts predated the changed sources. `make -C apps/backend build` refreshed
them, and the managed runner rebuilt the web and fixture artifacts for the
passing runs above.

