---
spec: docs/specs/ui/entity-reference-composer.md
decision: docs/decisions/2026-07-21-work-item-reference-search.md
created: 2026-07-21
status: implemented
---

# Implementation Plan: Entity Reference Composer

## Overview

Build a backend-normalized, workspace-scoped provider registry first, then add each external adapter without sharing mutable files. Registry and DTOs stay transport- and provider-neutral so native adapters can later be replaced by a plugin bridge with an explicit contribution and workspace permission. After central wiring and durable message metadata work, add the frontend search/client, `#` atom, composer interaction, sent-message chips, and desktop/mobile E2E.

## Backend

### Normalized search contract and Kandev tasks

Files:

- `apps/backend/pkg/api/v1/mentions.go` (new)
- `apps/backend/internal/mentions/types.go` (new)
- `apps/backend/internal/mentions/service.go` (new)
- `apps/backend/internal/mentions/handler.go` (new)
- `apps/backend/internal/mentions/service_test.go` (new)
- `apps/backend/internal/mentions/handler_test.go` (new)
- `apps/backend/internal/mentions/provider_tasks.go` (new)
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/task_mentions.go` (new)
- `apps/backend/internal/task/repository/sqlite/task_mentions_test.go` (new)
- `apps/backend/internal/task/service/service_mentions.go` (new)

Define `MentionProvider.Descriptor()` plus `MentionProvider.Search(ctx, SearchRequest)`, normalized groups/results/statuses, validation, deterministic group order, bounded concurrent all-settled execution, per-provider timeouts, safe errors, and the HTTP handler. Core registry code accepts arbitrary validated source/provider/kind IDs and uses descriptor labels/generic fallbacks; it contains no native-provider type switch. The registry owns provider identity and canonical `ref` construction, while adapters return bounded untrusted candidates with provider-local identity. Each unique `(provider, kind)` may also register a `ReferenceAuthorizer`; search filtering and later submission validation dispatch through that same provider-owned seam. Add a lightweight title-only task query with prefix-first ranking, workspace isolation, current-task exclusion, and no archived/ephemeral rows. Do not reuse full task listing because it hydrates repositories/sessions and applies PR-number augmentation.

### External provider adapters

Each adapter owns plain-query translation, workspace/config checks, stable identity mapping, exact destination/scope authorization for search and submission, safe failure classification, and focused tests in separate files:

- Jira: preserve immutable issue ID and generate escaped text/key JQL in `internal/jira/service_mentions.go`.
- Linear: use the existing structured `SearchFilter.Query` path.
- GitHub: preserve REST/node IDs and generate title-only issue/PR queries through workspace-scoped search.
- GitLab: include host + immutable object ID and add a workspace-scoped safe search wrapper.
- Azure DevOps: generate escaped WIQL for work items and add project-level active-PR listing/title filtering without repository-by-repository browser fan-out.
- Sentry: search every configured workspace instance through bounded organization discovery, include instance ID in scope, and escape free text.

Provider adapters live in `apps/backend/internal/mentions/provider_<name>.go`; provider-owned safe methods live beside their existing service/client code. No adapter registers routes or edits central backend composition.

### Message and queue reference metadata

Files:

- `apps/backend/internal/orchestrator/message_meta.go`
- `apps/backend/internal/task/handlers/message_handlers.go`
- `apps/backend/internal/task/handlers/message_handlers_test.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers_test.go`
- `apps/backend/internal/orchestrator/messagequeue/service.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_sqlite.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_sqlite_test.go`
- `apps/backend/internal/task/models/message_shell_output.go`
- `apps/backend/internal/task/models/message_shell_output_test.go`

Extract structural normalization/canonicalization into a neutral `internal/entityrefs` leaf, then validate, deduplicate, and persist `entity_references` for direct messages and queue add/update/drain. A conversation resolver derives trusted workspace from the persisted session/task; provider authorization dispatches through the registry and fails closed for unknown providers. Queue update replaces metadata instead of retaining deleted links. Project typed metadata to clients and build one sanitized agent-facing reference system block. Keep legacy `@task` context behavior intact.

### Composition and route wiring

Files:

- `apps/backend/internal/backendapp/types.go`
- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/backendapp/mentions.go` (new)
- `apps/backend/internal/backendapp/mentions_test.go` (new)

Construct adapters from available services, register them through stable provider descriptors, validate the workspace before fan-out, and mount `GET /api/v1/workspaces/:workspaceId/mentions/search`. Built-ins have deterministic descriptor order; the registry can also accept future dynamic descriptors without native-provider switches. Nil/unconfigured services must degrade to safe statuses. Add an aggregate integration test covering mixed success, disconnect, timeout, caps, and workspace isolation.

### Future plugin migration seam

This release does not change plugin manifests, protobufs, SDKs, or runtime. It does establish constraints for that later work:

- normalized result/reference DTOs are hand-mapped, versioned, additive, and independent of native structs, plugin wire models, and `structpb`, matching ADR 0043's public DTO rule;
- provider registration is dynamic and descriptor-driven, so a future plugin bridge registers providers without changing aggregation or React code; registry code injects provider identity and constructs canonical refs from validated plugin-local candidates;
- registry-issued source/provider IDs are stable across a native-to-plugin migration (or retain explicit aliases); brand-new plugin providers use reserved namespaced IDs and generic UI fallbacks;
- workspace scope is an explicit provider input, leaving the future bridge one place to enforce plugin workspace grants;
- provider timeout, cancellation, partial failure, URL validation, and result caps apply equally to native and future plugin-backed providers; and
- no task in this plan adds a `mentions.providers` manifest contribution, plugin workspace permission/grant, or `Plugin.SearchMentions` gRPC RPC. Those public wire and permission changes require their own plugin spec/ADR. Search flows Kandev-to-plugin and must not be inferred from plugin categories or reuse the plugin-to-Kandev Host data API/`api_read` capability.

## Frontend

### API client and search hook

Files:

- `apps/web/lib/types/entity-reference.ts` (new)
- `apps/web/lib/api/domains/mentions-api.ts` (new)
- `apps/web/lib/api/domains/mentions-api.test.ts` (new)
- `apps/web/hooks/use-entity-reference-search.ts` (new)
- `apps/web/hooks/use-entity-reference-search.test.ts` (new)

Add normalized types/client and a 250 ms debounced, abortable/generation-guarded hook. Search only with an active workspace and non-empty query, passing the active task as `exclude_task_id`. Preserve grouped partial results and a safe retryable aggregate error state.

### Composer and mobile interaction

Files:

- `apps/web/components/task/chat/entity-reference-types.ts` (new)
- `apps/web/components/task/chat/tiptap-entity-reference-extension.tsx` (new)
- `apps/web/components/task/chat/tiptap-entity-reference-suggestion.ts` (new)
- `apps/web/components/task/chat/entity-reference-menu.tsx` (new)
- `apps/web/components/task/chat/tiptap-input.tsx`
- `apps/web/components/task/chat/use-tiptap-editor.ts`
- `apps/web/components/task/chat/tiptap-helpers.ts`
- `apps/web/components/task/chat/popup-menu.tsx`
- `apps/web/components/task/chat/chat-input-body.tsx`
- `apps/web/components/task/chat/chat-input-container.tsx`
- `apps/web/components/task/chat/chat-input-area.tsx`
- `apps/web/components/task/passthrough-chat-composer.tsx`

Add a separate `entityReference` atom/plugin/menu; do not widen `MentionItem` or `ContextMention`. Explicitly enable `#` in task chat and Quick Chat and leave passthrough disabled. Remove new task discovery from `@` while retaining legacy node parsing. Serialize safe generated Markdown links, preserve draft JSON, carry reference metadata in history/reverse-search entries so only generated links rehydrate, and expose structured references from the editor handle.

Improve the shared popup's visual-viewport clamping, internal scroll owner, semantic listbox/option roles, pointer outside-dismiss, wrapping/truncation, and `min-h-11` touch rows.

### Mobile design contract

- **Desktop outcome / mobile entry:** typing `#` in the existing task/Quick Chat composer opens the same capability at both viewports.
- **Nearest exemplar:** shared `PopupMenu` and mobile slash-command composer supply anchored-above-caret selection; `MobilePickerSheet` is only a fallback if visual-viewport containment cannot preserve typing continuity.
- **Hierarchy / primary action:** provider and kind headings group a one-dimensional result list; selecting one row is the primary action.
- **Presentation:** anchored popover above the composer. This short, frequent typeahead must keep the software keyboard and caret active; a drawer would interrupt the typing loop.
- **Geometry:** visual viewport owns bounds, result body owns the only scroll, safe-area/keyboard clearance is explicit, rows are at least 44 px, and document horizontal overflow stays zero.
- **Shared logic:** query state, grouping, selection, serialization, and validation are shared; only viewport geometry changes.
- **Proof:** `mobile-entity-reference-composer.spec.ts` inserts and sends a reference by touch and asserts containment/no document overflow.

### Submission, queue, and sent rendering

Files:

- `apps/web/components/task/chat/chat-input-container.tsx`
- `apps/web/components/task/chat/use-chat-input-container.ts`
- `apps/web/components/task/chat/use-chat-input-state.ts`
- `apps/web/components/task/chat/chat-input-area.tsx`
- `apps/web/hooks/use-message-handler.ts`
- `apps/web/hooks/domains/session/use-queue.ts`
- `apps/web/lib/api/domains/queue-api.ts`
- `apps/web/lib/types/http.ts`
- `apps/web/lib/state/slices/session/types.ts`
- `apps/web/components/task/chat/messages/entity-reference-chip.tsx` (new)
- `apps/web/components/task/chat/messages/chat-message.tsx`
- `apps/web/components/task/chat/queued-ghost-message.tsx`

Replace the growing positional composer callback with a named submit payload, then carry `entity_references` through direct send and queue add/update. Render metadata-matched generated links as chips; unknown/missing metadata remains normal Markdown. Queue edits recompute references from surviving generated links. Preserve passthrough and existing context attachments without enabling `#` there.

## Tests

- Aggregator unit tests: deterministic groups, validation/caps, timeout/cancellation, partial errors, and sanitized statuses in `internal/mentions/service_test.go`.
- Task repository tests: title ranking, workspace isolation, current/archived/ephemeral exclusion in `task_mentions_test.go`.
- One focused adapter suite per provider for stable identity, workspace scope, escaping, fan-out bounds, cancellation, and error mapping.
- Handler/composition integration tests: HTTP request through real service registry with mixed fake adapters.
- Message/queue tests: direct persistence, queue update replacement, restart round-trip, drain, deduplication, unsafe metadata, metadata projection, and prompt-block sanitization.
- Frontend unit tests: URL construction, debounce/stale suppression, trigger boundaries, keyboard selection, serialization/history rehydration, viewport geometry, named submit payload, queue clearing, sent chip rendering, and legacy `@task` compatibility.

## E2E Tests

- `apps/web/e2e/tests/chat/entity-reference-composer.spec.ts`: task chat Arrow/Tab insertion, no auto-send, explicit send/clickable chip, partial provider result, draft restore, Quick Chat parity, and literal passthrough `#`.
- `apps/web/e2e/tests/chat/mobile-entity-reference-composer.spec.ts`: touch selection/send, 44 px row, keyboard/visual-viewport containment, internal scrolling, and no document horizontal overflow.

## Risks

- External typeahead can exhaust API quotas; debounce, cancellation, provider timeouts, and fan-out caps are mandatory. Any cache introduced by an adapter/client must include workspace and provider-connection scope in its key.
- GitHub/GitLab configuration history includes install-wide auth. Adapters must enforce workspace operational scope and validate the workspace before using shared credentials.
- Jira, Azure, GitHub/GitLab, and Sentry accept provider query languages today. Only provider-owned safe methods may translate the plain query.
- Azure PR and Sentry searches require extra scope discovery. Provider tests must prove bounded work and cancellation rather than silently omitting connected scopes.
- Hot frontend files already approach lint limits; new state, menus, atoms, and renderers stay extracted.
- Native-only switches or DTOs would make planned integration-to-plugin migration breaking. Conformance tests use an arbitrary fake provider descriptor to prove aggregation and frontend grouping remain generic.

## Verification

Targeted commands are recorded in each task file. Final integrated verification runs in this order:

```bash
make fmt
make typecheck test lint
cd apps/web && pnpm e2e:run tests/chat/entity-reference-composer.spec.ts tests/chat/mobile-entity-reference-composer.spec.ts
```

## Implementation Waves

Wave 1:

- [x] [task-01-core-and-task-search](task-01-core-and-task-search.md)

Wave 2 (parallel after Task 01):

- [x] [task-02-jira-provider](task-02-jira-provider.md)
- [x] [task-03-linear-provider](task-03-linear-provider.md)
- [x] [task-04-github-provider](task-04-github-provider.md)
- [x] [task-05-gitlab-provider](task-05-gitlab-provider.md)
- [x] [task-06-azure-provider](task-06-azure-provider.md)
- [x] [task-07-sentry-provider](task-07-sentry-provider.md)
- [x] [task-08-message-reference-metadata](task-08-message-reference-metadata.md)

Wave 3:

- [x] [task-09-backend-composition](task-09-backend-composition.md)

Wave 4 (sequential):

- [x] [task-10-frontend-search-client](task-10-frontend-search-client.md)
- [x] [task-11-composer-reference-ui](task-11-composer-reference-ui.md)
- [x] [task-12-message-reference-ui](task-12-message-reference-ui.md)

Wave 5:

- [x] [task-13-e2e-and-verification](task-13-e2e-and-verification.md)
