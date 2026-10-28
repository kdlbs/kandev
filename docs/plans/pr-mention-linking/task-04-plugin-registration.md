---
id: "04-plugin-registration"
title: "Plugin registration and conformance"
status: pending
wave: 4
depends_on:
  - "03-queued-mentions"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-002
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-003
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-004
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-005
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-006
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-007
acceptance_criteria:
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-002.1
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-002.2
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-002.4
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-003.1
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-003.2
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-003.3
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-003.4
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-004.1
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-004.2
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-004.3
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-004.4
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-005.1
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-005.2
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-005.3
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-006.1
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-006.2
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-006.3
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-006.4
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-007.1
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-007.2
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-007.3
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-007.4
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-007.5
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-007.6
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-007.7
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-007.8
system_design:
  - ../../specs/integrations/system-design/task-change-registration.md
  - ../../specs/integrations/system-design/task-change-registration-plugins.md
---

# Task 04: Plugin registration and conformance

## Summary

Deliver the complete registration path for capability-compatible plugins through the existing autocomplete, shared tools, and native task PR surfaces.
Use plugin-owned association persistence and exact receipt reconciliation; do not assume a transaction spans host and plugin stores.

## In scope

- Add versioned manifest registration declarations, SDK DTOs, validation, active-owner routing and three typed backend action adapters.
- Reuse `MentionSourceBridge`; map each declared reference source to an exact repository/review provider without naming conventions or first-match fallback.
- Extend neutral management/read/list projections, URL resolution, plugin-only catalog visibility, and shared guidance. Keep built-in tool fields compatible.
- Add plugin operation receipts and generation-bound recovery to host registration intents; require plugin transactional tombstones and revision checks.
- Persist nullable resolution fences without blocking authorized message acceptance. Add the typed `create_if_absent` precondition for unresolved recovery.
- Keep Retry bound to its original fence and operation ID. Distinguish failed resolution from uncertain mutation in API and UI state.
- Add the browser SDK management helper and existing review-provider invalidation/provenance fields; route capable plugin manual unlink through the same coordinator.
- Add a disposable fixture with Bitbucket Cloud, Data Center context-root and Forgejo-shaped identities; test headless and browser flows.
- Document adoption and minimum host/SDK versions in manifest/authoring/API guides. Correct the old composer design's future-bridge wording.
- Reconcile existing task-change MCP contracts and all package results. Promote shared specs only after all four work orders pass.

## Out of scope

Production Bitbucket/Forgejo plugin implementation or publication, new provider-specific host clients, repository attachment, a generic association database, and new MCP tools.
The fixture proves host compatibility. It does not certify an installed production plugin release or grant missing automation capability.

## Acceptance

1. A capable plugin completes initial/direct/queue mention and agent URL registration through the shared flow; partial/older providers retain truthful context-only behavior.
2. Exact identity, receipts and tombstones withstand lost replies, unlink races, restart and plugin-generation changes without execution/comparison effects. Resolver failure preserves the message; recovery cannot acquire restoration authority.
3. Existing plugin lists/counts/unlink and shared feedback/provenance work on desktop/phone; SDK documentation and conformance evidence describe adoption accurately.

## ASCII UI preview

UI-06 from the [combined preview](plan.md#ui-06-plugin-autocomplete-and-capability-feedback):

```text
Fix CI on #auth
  Bitbucket Cloud: team/app #123
  Forgejo:        team/app #123
Selected: [Forgejo #123 | Link on send v]
Sent:     [Forgejo #123] Linked PR #123
Resolver unavailable: Not linked [Retry]
Mutation reply lost:  Status unknown [Retry]
Older plugin: Context only [Why unavailable?]
```

Phone reuses UI-03's drawer with 44-pixel controls and the same Link on send / Context only choices.
Provider and connection identity remain available in accessible labels/details. Right-click is not required.
Unavailable results never show a false success or erase a last-known task count.
Maps to 007.1, .2, .6, .7, .8 and 006.*; reuse existing tokens and localized controls.

## Verification

Run from the repository root. Install web workspace dependencies once if absent: `(cd apps && pnpm install --frozen-lockfile)`.
New test files below are implementation deliverables. Use the fixture's durable test store, not real provider credentials or the live Kandev database.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/plugins/manifest ./internal/plugins ./pkg/pluginsdk ./internal/backendapp ./internal/mcp/handlers ./internal/mcp/server ./cmd/plugin-fixture -run 'Registration|Mention|Reference|RepositoryProvider|Action|TaskChange' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/handlers ./internal/orchestrator/handlers ./internal/orchestrator/messagequeue ./internal/task/repository/sqlite -run 'PluginRegistration|Registration' -count=1)
(cd apps/backend && test -n "$KANDEV_TEST_POSTGRES_DSN" && go test -trimpath -race -tags fts5 ./internal/task/repository/sqlite -run 'Postgres.*Registration|Registration.*Postgres' -count=1 -v)
(cd apps/packages/plugin-sdk && pnpm test && pnpm run typecheck)
(cd apps/web && pnpm exec vitest run lib/plugins/sdk-contract.test.ts lib/plugins/registry.review-providers.test.ts lib/plugins/change-request-registration.test.ts components/task/chat/reference-registration.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium -- tests/plugins/pr-registration.spec.ts tests/chat/pr-mention-linking.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome -- tests/plugins/mobile-pr-registration.spec.ts tests/chat/mobile-pr-mention-linking.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the package's documented PR-documentation contract preflight after updating results. Confirm every new backend test name matches `Registration`.
Conformance must prove registration with no browser session, not only JavaScript callback behavior.
Record explicit non-skipped PostgreSQL receipt results and rendered comparisons against UI-06/03/05.
Add `TestPluginRegistrationResolutionUnavailable` and `TestPluginRegistrationAdmissionFence` with controlled unlink/resolve/admit/mutate interleavings.
Cover resolver outage, restart, credential recovery, null fences with absent/active/detached rows, and delayed receipt replies after a second unlink.
Assert the accepted message remains unchanged and that Retry never advances a fence. Only a fresh authorized Link again can request restoration.
Desktop/mobile E2E must distinguish Not linked, Status unknown, and Unlinked, and prove recovery does not resend the message.
Do not require a production plugin download to validate the host contract.

## Files likely touched

- `apps/backend/internal/plugins/manifest/{manifest,validate}.go`, manifest tests, stored plugin declarations and registry capability projections.
- `apps/backend/internal/plugins/mentions.go`, `repository_provider.go`, `repository_provider_inspect.go`, `action_handlers.go`, and new registration bridge/tests.
- `apps/backend/pkg/pluginsdk/` typed registration action DTOs and conformance helpers over existing `HandleAction`.
- `apps/backend/internal/backendapp/task_change_link_coordinator.go`, read adapters, wiring, and new plugin-registration tests.
- `apps/backend/internal/mcp/{handlers,server}/` management/read schema, tool catalog and guidance availability.
- `apps/backend/internal/task/repository/sqlite/` registration-intent fields/recovery and database tests.
- `apps/backend/cmd/plugin-fixture/` backend fixture, durable test association store, manifest and UI bundle.
- `apps/packages/plugin-sdk/src/` and `test/` for capability, association DTOs and `host.changeRequests.manage`.
- `apps/web/lib/plugins/{types,host-api,registry}.ts`, review-provider shared resources, registration invalidation and new tests.
- New `apps/web/e2e/tests/plugins/pr-registration.spec.ts` and `mobile-pr-registration.spec.ts`.
- `docs/public/plugins-manifest.md`, `plugins-authoring.md`, `docs/plans/plugins/PLUGIN-API.md`, MCP/composer contract links and package status/results.

## Dependencies

Task 03, including Tasks 01 and 02. The base resolver/intent/retry/UX contract must exist before a plugin adapter implements it.
Both system designs listed above are required inputs. Other systems retain their own requirements; link them instead of copying IDs.

## Risks

A plugin mutation can commit after host timeout. Treat cancellation as uncertain and reconcile by operation receipt.
Plugins with only browser callbacks cannot provide headless association; retain their legacy flows without advertising automatic support.
Host state key/value methods do not provide cross-key transactions. Plugin implementations need durable atomic receipt/tombstone storage.
Uninstall deletes plugin data. Never replay historical intents automatically into a replacement installation.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/task-change-registration.md) and both listed system designs.
- [Investigation](investigation.md), [validation matrix](validation-matrix.md), [Plugin API](../plugins/PLUGIN-API.md).
- Existing `mentions_test.go`, `repository_provider_inspect_test.go`, `action_access_test.go`, and browser SDK/review-provider tests.
- Root/backend/web guidance, specification/planning skills, and mobile-parity skill.

## Results

Pending. This turn updates documentation only; no plugin, SDK, backend, frontend, migration or test implementation ran.
