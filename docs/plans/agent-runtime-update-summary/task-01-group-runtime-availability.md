---
id: "01-group-runtime-availability"
title: "Deliver grouped runtime availability"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-NOTIFY-003
acceptance_criteria:
  - AC-AGENTS-RUNTIME-NOTIFY-003.1
  - AC-AGENTS-RUNTIME-NOTIFY-003.2
  - AC-AGENTS-RUNTIME-NOTIFY-003.3
  - AC-AGENTS-RUNTIME-NOTIFY-003.4
  - AC-AGENTS-RUNTIME-NOTIFY-003.5
  - AC-AGENTS-RUNTIME-NOTIFY-003.6
  - AC-AGENTS-RUNTIME-NOTIFY-003.7
  - AC-AGENTS-RUNTIME-NOTIFY-003.8
  - AC-AGENTS-RUNTIME-NOTIFY-003.9
system_design:
  - ../../specs/agents/system-design/runtime-update-summary.md
---

# Task 01: Deliver grouped runtime availability

## Summary

Replace concurrent agent availability notices with one delayed summary across subscribed channels.
Deliver collector, provider claims, compatible payloads, localized presentation, and navigation as one tested vertical change.

## In scope

- Controller collection and revalidation under the existing background lifecycle.
- Batch notifier and per-member delivery claims, rollback, and replay.
- Typed summary payload, singular compatibility, locale catalogs, and Settings navigation.
- Focused desktop/phone regressions and public runtime-notification guidance.

## Out of scope

New preferences, generic rule engines, feature flags, schema changes, an outbox, and runtime activation changes.

## Acceptance

- Startup/reconnect bursts produce one delayed availability message; obsolete members disappear and terminal outcomes bypass collection.
- Each recipient/provider receives only newly claimed versions; failed deliveries remain replayable without repeating successful members.
- The localized summary works on desktop and phone, reaches the expanded runtime section, and preserves singular/release payload compatibility.

## ASCII UI preview

UI-01: App route after startup checks. See the [full preview and states](plan.md#ascii-ui-preview).

```text
+----------------------------------+
| 4 agent runtime updates available |
| Review versions in Settings.     |
| [Review updates]                 |
+----------------------------------+
```

One compact notice on desktop and phone; phone action is at least 44px.
Direct navigation opens Settings details. No availability toast appears during collection or for an empty group.
This view covers AC-AGENTS-RUNTIME-NOTIFY-003.2, 003.3, 003.4, and 003.8.

## Verification

Use TDD for changed logic. Run commands from the repository root.
If workspace dependencies are absent, first run `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/backend && go test -trimpath -race ./internal/agent/settings/controller ./internal/notifications/service ./internal/notifications/providers)
(cd apps/backend && go test ./internal/backendapp -run 'Test(NewE2ERuntimeUpdateHooksRequiresMockRuntimeAndVersion|E2ERuntimeUpdateHooksReleaseAndCancel)$' -count=1)
make -C apps/backend build
(cd apps/web && pnpm exec vitest run hooks/use-update-available-toast.test.ts components/update-available-toast-bridge.test.tsx lib/ws/handlers/notifications.test.ts lib/state/slices/ui/ui-slice.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run build:vite)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/agent-runtime-notifications.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-agent-runtime-notifications.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Managed E2E runs rebuild and run sequentially. Add every newly created unit suite to this verification record if its path falls outside these commands.
Update `docs/public/agents-and-profiles.md` at implementation time, following its existing how-to structure.

## Files likely touched

- `apps/backend/internal/agent/settings/controller/runtime_update_background.go`
- `apps/backend/internal/agent/settings/controller/runtime_update_notifications.go`
- `apps/backend/internal/agent/settings/controller/runtime_update_notifications_test.go`
- Proposed collector helper and `runtime_update_summary_test.go` in the same controller package
- `apps/backend/internal/backendapp/main.go` if lifecycle wiring requires an adjustment
- `apps/backend/internal/notifications/service/runtime_updates.go` and proposed `runtime_update_summary_test.go`
- `apps/backend/internal/notifications/providers/provider.go`, `local.go`, and `local_test.go`
- Proposed runtime member model under `apps/backend/internal/notifications/models/`
- `apps/web/lib/types/backend.ts`, `lib/state/slices/ui/types.ts`, `ui-slice.ts`, and `ui-slice.test.ts`
- `apps/web/lib/ws/handlers/notifications.ts` and `notifications.test.ts`
- `apps/web/hooks/use-update-available-toast.ts` and `.test.ts`
- `apps/web/components/update-available-toast-bridge.tsx` and `.test.tsx`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/agents.json` and generated pseudo catalog
- `apps/web/e2e/tests/settings/agent-runtime-notifications-helpers.ts` and desktop/phone specs
- `docs/public/agents-and-profiles.md`
- Paired summary specs and this package's status/results

## Dependencies

None. Read existing runtime-notification requirements/design, nearby test doubles, backend/frontend guidance, and the paired summary design before implementation.

## Risks

Avoid summary-level deduplication, delayed outcomes, timers outside controller shutdown, and rendering a grouped message as a Kandev release.
Preserve existing provider ownership and prefer shared logic for desktop/phone presentation.

## Parallelism

`sequential`

## Inputs

- [Summary requirements](../../specs/agents/requirements/runtime-update-summary.md)
- [Summary design](../../specs/agents/system-design/runtime-update-summary.md)
- [Existing runtime design](../../specs/agents/system-design/runtime-update-notifications.md)
- Existing runtime notification, local provider, toast, bridge, and settings E2E suites

## Results

- TDD red/green evidence: the controller summary test first observed two individual immediate notices instead of a windowed batch. The toast hook test first observed no grouped Settings action. After implementation and the entrance-animation wait, both regressions passed.
- `go test -trimpath -race ./internal/agent/settings/controller ./internal/notifications/service ./internal/notifications/providers` passed. This includes the controller timer/replay/revalidation/cancellation/outcome tests and notification per-recipient/per-provider claim and retry tests.
- `make -C apps/backend build` and `pnpm --filter @kandev/web build:vite` passed. Vite emitted the existing chunk-size, dynamic-import, and advancedChunks warnings; Darwin helper artifacts noted unavailable codesign tools.
- The four focused Vitest files passed (76 tests). `pnpm run typecheck`, `i18n:zh-hant`, `i18n:pseudo`, `i18n:check`, and `i18n:ratchet` passed.
- Desktop runtime-notification E2E passed 5/5 cases, including 390px and 767/768px boundaries. Mobile Chrome passed 2/2 cases. Focused capture runs produced and visually checked a desktop summary frame and a phone summary frame against UI-01. The phone action measured at least 44px; both notices remained within the viewport and opened the expanded Settings section.
- `node --test scripts/validate-public-docs.test.mjs` passed 62/62 tests; `node scripts/validate-public-docs.mjs` validated 47 published pages. `python3 scripts/list-docs.py validate` validated 348 decisions and 1338 specifications; `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check` passed after the initial implementation.
- Review remediation: terminal outcomes now dispatch without waiting for the collector, while bounded per-runtime coalescing keeps availability producers nonblocking. Revalidation refreshes current version and display metadata while retaining the queued occurrence identity. A regression also verifies a status carrying both a terminal outcome and current availability preserves both paths.
- The new backend-to-UI desktop and phone scenarios keep a real WebSocket client connected across backend restart, verify immediate outcome delivery during collection, assert no early availability toast, and confirm one final summary with no repeated member claims after reload/replay. The injected-summary case remains for presentation coverage.
- Final remediation verification passed: controller/service/provider race tests; focused backendapp E2E-hook tests; backend build; desktop runtime-notification E2E (6/6); mobile Chrome runtime-notification E2E (3/3); and `git diff --check`. The managed Vite build emitted its existing chunk-size and dynamic-import warnings, and Darwin helper artifacts noted unavailable codesign tools.
- After the final change preserving availability when a status also carries a terminal outcome, the real backend startup/reconnect test passed against the final backend build on Chromium (1/1) and mobile Chrome (1/1).
- Additional PR-review remediation on 2026-10-05 moved revalidation and provider sends to a lifecycle-owned worker with a bounded, coalescing per-runtime queue. A deterministic barrier test verifies that a later group is queued and a terminal outcome is delivered while the first provider send is blocked. Removing all pending members no longer resets the first window deadline, and shutdown cancels active sends and clears pending work.
- System-provider grouped copy now follows its configured locale or the host locale. Tests cover all shipped locales and English fallback. The backend readiness regression uses `testing/synctest`; the manual-runtime test runs through the background collector and asserts its delayed Kimi summary. The WS summary fixture now carries both members named by its count.
- The startup/reconnect E2E now measures early delivery from before backend restart, starts its outcome wait after reconnect, and gives replay-after-reload enough time for reload overhead. Presentation injection coverage remains.
- Additional verification passed: controller/service/provider race tests; backendapp E2E-hook tests; backend build and lint; focused notification Vitest tests (76/76); web typecheck; focused ESLint and Prettier; desktop runtime-notification E2E (6/6); mobile Chrome runtime-notification E2E (3/3); and `git diff --check`.
