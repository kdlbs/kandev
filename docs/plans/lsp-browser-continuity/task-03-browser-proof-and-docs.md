---
id: "03-browser-proof-and-docs"
title: "Browser proof and public docs"
status: pending
wave: 3
depends_on:
  - "01-runtime-lsp-leases"
  - "02-editor-reconnection"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-LSP-FILE-INTELLIGENCE-002
acceptance_criteria:
  - AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.1
  - AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.2
  - AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.3
  - AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.4
  - AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.5
  - AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.6
  - AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.7
  - AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.8
  - AC-PLATFORM-LSP-FILE-INTELLIGENCE-002.9
system_design:
  - ../../specs/platform/system-design/lsp-file-intelligence-01.md
  - ../../specs/platform/system-design/lsp-file-intelligence-02.md
---

# Task 03: Browser proof and public docs

## Summary

Prove browser close and return against a real task runtime and a deterministic fake language server. Update public guidance to describe retained processes, capacity, Stop, and restart recovery.

## In scope

- Extend the existing fake LSP fixture with a count or process marker that distinguishes reattachment from a fresh process and initialize.
- Enable the release flag explicitly in the E2E fixture. Add desktop E2E for page close/reopen after a completed agent turn and reaper interval, fresh diagnostics and progress, duplicated-tab isolation, explicit Stop, editor-idle release, detached-lease eviction, all-attached capacity, and task-host restart. Add tablet reattachment coverage and retain the phone no-socket assertion. Include a disabled-flag regression of the existing browser-owned lifecycle.
- Update the existing public developer-tools, configuration, WebSocket API, and feature-status text where the previous browser-owned lifecycle is documented.

## Out of scope

- New LSP capabilities, executor support, mobile phone LSP, or a new status surface.

## Acceptance

1. A desktop browser closes and reopens on the task; the same fake server process remains, the editor reaches ready with current-file diagnostics and project status, and no second initialize occurs.
2. A second active window and duplicated tab remain independent, idle release and capacity eviction free task-host resources, explicit Stop and host restart have the specified effects, and the tablet drawer regains status while phone opens no LSP socket.
3. Public docs accurately describe retained-lease capacity, Stop, restart recovery, and transport versus process-exit statuses; focused docs validators pass.

## ASCII UI preview

`UI-01: Desktop active Monaco file`, `UI-02: Coarse-pointer tablet drawer`, and `UI-03: Phone file viewer` use the [combined preview](plan.md#ascii-ui-preview) (AC .2, .3, .7).

```text
Desktop: [Go LSP: Reconnecting] -> [Go LSP: Ready + project report]
Tablet:  [Go LSP] -> bottom drawer -> Ready + project report
Phone:   main.go -> file content; no LSP control
```

No new navigation is required. Test the existing desktop and tablet surfaces and the phone absence contract.

## Verification

```bash
(cd apps/web && pnpm e2e:run tests/lsp/lsp-file-intelligence.spec.ts -- --grep "retains|reattaches|disconnects|evicts|releases|capacity")
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/lsp/mobile-lsp-file-intelligence.spec.ts -- --grep "reattaches|without starting")
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

The implementation must use test names matching the focused grep expressions and confirm Playwright discovers the intended scenarios. The managed runner builds production frontend and backend assets for each command.

## Files likely touched

- `apps/web/e2e/fixtures/fake-lsp-server.mjs`
- `apps/web/e2e/tests/lsp/lsp-file-intelligence.spec.ts`
- `apps/web/e2e/tests/lsp/mobile-lsp-file-intelligence.spec.ts`
- `apps/web/e2e/tests/lsp/lsp-e2e-helpers.ts`
- `apps/web/e2e/fixtures/backend.ts` or the nearest per-spec release-flag override
- `docs/public/developer-tools.md`
- `docs/public/configuration.md`
- `docs/public/websocket-api.md`
- `docs/public/feature-status.md` if its current lifecycle summary changes

## Dependencies

Tasks 01 and 02 must complete so the E2E scenarios exercise the final protocol and UI.

## Risks

- Browser page close may race process teardown; the fake server marker must prove the same process survived rather than merely a fast restart.
- The tablet test must exercise its actual drawer and touch action; the phone test must confirm no socket is opened.
- The public config key name remains unchanged even though its resource count becomes retained leases.
- A retained lease pins the whole Local PC or Docker task host until release, eviction, or task stop; public guidance should make this cost and fresh-start-after-eviction behavior clear.

## Parallelism

`sequential`

## Inputs

- [Plan and preview](plan.md)
- `REQ-PLATFORM-LSP-FILE-INTELLIGENCE-002` and both linked system designs.
- Completed Task 01 and Task 02 results.

## Results

Pending.
