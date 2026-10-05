---
id: "03-recovery-proof"
title: "Prove native and desktop/mobile recovery"
status: complete
wave: 3
depends_on:
  - "02-hidden-continue"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-001
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-002
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-003
acceptance_criteria:
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.30
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.31
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.2
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.3
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.6
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.7
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.1
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.2
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.3
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.5
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.1
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.2
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.3
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.4
system_design:
  - ../../specs/platform/system-design/provider-error-recovery.md
  - ../../specs/platform/system-design/provider-error-recovery-cursor.md
  - ../../specs/platform/system-design/provider-interruption-continuation.md
---

# Task 03: Prove native and desktop/mobile recovery

## Summary

Prove that literal continuation works with native Cursor after completed work,
and that desktop/phone show accurate status without the internal prompt. Update
experimental metadata and public docs; keep rollout opt-in and record actual proof.

## In scope

- Isolated native `continue` probes for completed shell and file-write work,
  subsequent interruption, exact identity restoration/reuse, meaningful follow-up
  response, and absence of host original-prompt replay. Record CLI version and
  sanitized evidence without user data. Never probe the reported failed session.
- Extend mock completed-tools/resource-exhaustion scenarios and existing E2E
  helpers to inspect exact `continue`, original/continuation dispatch counts,
  category, and live/restored native identity independently of response markers.
- Desktop and mobile user outcomes: one owned notice, bounded shared attempts,
  Cancel/human supersession, mixed pending refusal, real error/category in technical
  details, hidden internal prompt after reload/pagination/second viewer, and visible
  ordinary user `continue`. No frontend text-based hiding.
- Existing feature registry/settings description and locale keys describe
  successful completion rather than read-only work. Preserve default-off keys,
  restart metadata, typed gates, and disabled-path behavior. Translate new copy
  in all seven supported languages and generate Traditional Chinese catalogs.
- Public operator/task documentation and scoped engineering guidance when the
  V2 protocol description changes. Promote amended spec statuses only after every
  work order's required evidence passes; completed old plans remain historical.

## Out of scope

Promotion, live flag changes/restarts, new visual surfaces, other provider/native
implementations, screenshots or media beyond focused test artifacts, and broad QA.

## Acceptance

- Native probes demonstrate meaningful continued work with exact `continue` and
  the same saved identity after completed shell/write tools. A skipped native test
  or accepted-prompt-only assertion does not complete this work order.
- Desktop and phone show actual resource exhaustion and one current recovery
  status, with no internal user row in DOM/API/prompt history across reload,
  pagination, and another viewer. Human `continue` remains visible and Cancel is
  reachable with a phone hit target at least 44px, no horizontal overflow.
- Enabled/disabled behavior, shared budget, old-wire refusal, and ordinary runtime
  reuse regressions pass. Docs and localized metadata match the new behavior;
  profile defaults remain off and no live instance is changed.

## ASCII UI preview

UI-01 from the [plan](plan.md#ascii-ui-preview), task Chat, waiting then continuing:

```text
Desktop: [Completed results]
         Cursor: Resource exhausted. Attempt 1/5. 5s   [Cancel]
         [Continued agent response; no user continue row]
Phone:   [Completed results]
         Cursor: Resource exhausted. Attempt 1/5
         [ Cancel (44px minimum) ]
         [Continued agent response]
         [Existing composer/safe area]
```

Localized wording is illustrative. Reuse current inline continuation UI, one Chat
scroll owner, and desktop density. Cover AC `001.6`, `001.7`, and `003.1`-`.4`.

## Verification

From the repository root; native commands require existing authenticated Cursor
test credentials. Use isolated owned fixtures, not the developer's live task.

```bash
cursor-agent --version
(cd apps/backend && KANDEV_TEST_CURSOR_NATIVE_CONTINUATION=1 go test -tags fts5 -race ./internal/agent/runtime/lifecycle -run '^TestCursorNative(CompletedToolContinue|ConversationRestore|AbruptDisconnectRestore|SessionResume)$' -count=1 -v)
(cd apps/backend && go test -tags fts5 -race ./internal/runtimeflags ./internal/common/config ./internal/profiles -count=1)
(cd apps/web && pnpm exec vitest run lib/state/slices/features/features-contract.test.ts components/task/chat/messages/interruption-recovery-feedback.test.ts components/task/chat/messages/action-message.test.tsx)
(cd apps/web && pnpm e2e:run --shards 1 --project chromium tests/session/provider-interruption-continuation.spec.ts tests/session/transient-turn-runtime-continuity.spec.ts tests/session/transient-retry.spec.ts)
(cd apps/web && pnpm e2e:run --shards 1 --project mobile-chrome tests/session/mobile-provider-interruption-continuation.spec.ts tests/session/mobile-transient-turn-runtime-continuity.spec.ts tests/session/mobile-transient-retry.spec.ts)
(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:check && pnpm run typecheck && pnpm run lint)
make -C apps/backend lint
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Add `TestCursorNativeCompletedToolContinue` in the existing native probe suite;
record its assertion result and installed version. Managed E2E rebuilds runtime/web
artifacts; run desktop and mobile sequentially. The new scenario names must be
discovered in the selected project. A fresh worktree requires frozen pnpm install
from `apps/` first. All Task 01/02 regression commands remain required after any
production remediation introduced by this proof work.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/cursor_native_continuation_test.go` and helpers
- `apps/backend/cmd/mock-agent/interruption_continuation.go` and scenarios/tests
- `apps/web/e2e/helpers/provider-interruption-continuation.ts`
- `apps/web/e2e/tests/session/provider-interruption-continuation.spec.ts`
- `apps/web/e2e/tests/session/mobile-provider-interruption-continuation.spec.ts`
- Retained-turn desktop/mobile specs and action-message component tests as needed
- `apps/backend/internal/runtimeflags/registry.go` metadata only
- `apps/web/components/task/chat/messages/` semantic label mapping only where needed
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-tw,zh-hk,ja,ko,pseudo}/` affected catalogs
- `docs/public/tasks-and-workflows.md`, `docs/public/operations.md`, and existing relevant agent page
- `apps/backend/internal/agentctl/AGENTS.md` when documenting the new wire boundary
- This plan's results and sanitized `compatibility-evidence.md`

## Dependencies

Tasks 01 and 02 complete. Read `/e2e`, `/mobile-parity`, `/docs-maintainer`, and
`/runtime-feature-flags`; native authentication is a verification prerequisite.

## Risks

Native model completion must be checked without assuming provider exactly-once
behavior. A mocked success string or stale message can conceal a missing follow-up.
No result may be copied from the completed V1 plan as V2 evidence.

## Parallelism

`sequential`

## Inputs

- UI-01, amended requirements/designs, native compatibility instructions.
- Existing desktop/mobile continuation helpers, causal waits, prompt-history
  projection, registry/profile contract tests, and public documentation structure.

## Results

Native proof now requires completed ACP tool outcomes, preserved conversation
identity, and the requested final response after literal `continue`. Both edit
and shell probes and the existing restore/disconnect/resume matrix pass on CLI
`2026.10.01-e373342`; the isolated shell fixture approves only its exact command.

Review remediation fixes unknown/mixed-pending fixtures, requires exact wire
`continue`, proves automatic messages stay hidden across reload and a second
viewer, and verifies that a human `continue` remains visible. All 15 desktop
continuation E2E cases pass. Resource-exhaustion label coverage uses a synthetic
status fixture on desktop and phone and localized catalogs in every supported
language. The plan records native history limits and sandbox-only baseline test
failures alongside the passing targeted race, frontend, lint, and docs checks.

Final delivery verification on the current main base: six focused desktop cases
and both phone cases pass in the managed Docker runner with retries disabled.
Fresh desktop and phone captures show the localized resource-exhaustion reason,
continuation countdown, and Cancel action. The phone target is at least 44px and
the document has no horizontal overflow. Captures use synthetic seeded status
data; raw diagnostic classification and admission are verified by backend tests.
