---
id: "01-ack-gated-session-hydration"
title: "Gate session hydration on subscribe acknowledgement"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-001
acceptance_criteria:
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-001.1
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-001.2
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-001.3
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-001.4
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-001.5
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-001.6
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-001.7
  - AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-001.8
system_design:
  - ../../specs/platform/system-design/session-subscription-recovery.md
---

# Task 01: Gate session hydration on subscribe acknowledgement

## Acceptance

- A transcript hydration request cannot overtake the server's successful
  `session.subscribe` registration, including shared, reconnect, and delayed
  retry paths.
- Existing ref-counted consumers still subscribe/unsubscribe correctly, and
  failed or disconnected requests do not leave unhandled rejections or stale
  readiness state.
- The existing session auto-start and follow-up-response flows no longer need a
  reload to recover a missed subscription event; unrelated bounded startup
  waits remain intact.

## Verification

From a fresh worktree when dependencies are absent:

```bash
cd apps && pnpm install --frozen-lockfile
```

Targeted checks:

```bash
cd apps && pnpm --filter @kandev/web test -- --run lib/ws/client.test.ts hooks/domains/session/use-session-messages.test.ts hooks/domains/session/use-session-subscription-retry.test.ts
cd apps/web && pnpm run typecheck
cd apps/web && pnpm exec eslint --max-warnings 0 lib/ws/client.ts lib/ws/client.test.ts hooks/domains/session/use-session-messages.ts hooks/domains/session/use-session-message-fetch.ts hooks/domains/session/use-session-messages.test.ts hooks/domains/session/use-session-subscription-retry.ts hooks/domains/session/use-session-subscription-retry.test.ts e2e/pages/session-page.ts e2e/tests/chat/mobile-unread-divider.spec.ts e2e/tests/chat/unread-divider.spec.ts
cd apps/backend && go test -race -run 'TestHandleSessionSubscribe|TestBroadcastToSession|TestHandleSessionDataRefresh' ./internal/gateway/websocket/...
cd apps/web && pnpm e2e:run --project chromium tests/session/auto-start-session.spec.ts tests/session/session-recovery.spec.ts
```

If the page-object change affects mobile behavior, also run:

```bash
cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-pause-resume-recovery.spec.ts
```

## Files likely touched

- `apps/web/lib/ws/client.ts`
- `apps/web/lib/ws/client.test.ts`
- `apps/web/hooks/domains/session/use-session-messages.ts`
- `apps/web/hooks/domains/session/use-session-message-fetch.ts`
- `apps/web/hooks/domains/session/use-session-messages.test.ts`
- `apps/web/hooks/domains/session/use-session-subscription-retry.ts`
- `apps/web/hooks/domains/session/use-session-subscription-retry.test.ts`
- `apps/web/e2e/pages/session-page.ts`
- `apps/web/e2e/tests/chat/mobile-unread-divider.spec.ts`
- `apps/web/e2e/tests/chat/unread-divider.spec.ts`

## Dependencies

None.

## Parallelism

Sequential. The client readiness contract is shared by every implementation
file and must be tested before E2E workaround cleanup.

## Inputs

- `docs/specs/platform/requirements/session-subscription-recovery.md`
- `docs/plans/session-subscription-recovery/plan.md`
- Existing backend acknowledgement/snapshot behavior in
  `apps/backend/internal/gateway/websocket/client.go` and
  `apps/backend/internal/backendapp/helpers.go`
- Existing ref-counted subscription behavior in
  `apps/web/lib/ws/client.ts`

## Results

Implemented the acknowledgement-gated subscription contract across the web
client and session message hooks. Initial hydration, reconnect
re-registration, and delayed unknown-session retries now use tracked subscribe
requests; ref-counted consumers share readiness, and cleanup prevents late
hydration from mutating state. The race-specific reload fallbacks were removed
from the session page object and affected chat specs. No backend production
change was needed because the existing gateway acknowledgement ordering was
already correct.

Verification:

- Focused web tests: 3 files, 46 tests passed.
- Web typecheck passed.
- Focused ESLint passed with zero warnings.
- Backend gateway race tests: 6 passed.
- Desktop Chromium session E2E: 5 passed.
- Mobile Chrome recovery E2E: 2 passed.
- `git diff --check` passed.

## PR fixup results (2026-10-10)

The gateway preserves socket order for legacy session membership changes.
Initial snapshot loading remains asynchronous, and independent request handlers remain concurrent.
This repairs the existing shared membership contract during session promotion.
The live authorization barrier test failed before the repair and passed afterward.
The complete gateway package passed `go test -trimpath -race ./internal/gateway/websocket -count=1`.
The changed-scope gateway Go lint reported zero issues.
Browser verification: `cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/chat/quick-chat-cancel-palette.spec.ts -- --retries=0` passed all three tests after a fresh runtime and web build.

CI documentation coverage required migration of the legacy frontmatter.
The work order now declares its existing requirement, all eight acceptance criteria, and system design.
The linked plan declares the same requirement and design.
The trusted coverage evaluator passed the complete captured PR file set after this metadata repair.
