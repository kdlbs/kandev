# 0044: Typed Utility Chats Share the Quick Chat Session Model

**Status:** accepted
**Date:** 2026-07-14
**Area:** backend, frontend

## Context

Ordinary quick chats and configuration chats both use ephemeral tasks and the shared chat shell,
but they maintain separate frontend tab stores and presentation containers. Configuration tabs
exist only in memory, so reload makes their still-persisted tasks unreachable, and their fixed
popover constrains reports and inline clarification questions. Config-mode tasks are intentionally
excluded from ordinary quick-chat expiration, making client-only tab closure an unbounded orphan
risk.

## Decision

All user-facing utility conversations use one typed Quick Chat session model with
`kind: "chat" | "config"` and one tab store. `QuickChatContent` remains the single
message/composer/clarification renderer. The responsive Quick Chat modal is the primary large
surface; Settings also provides a compact floating configuration presentation over the same typed
sessions. Expanding the floating presentation activates the same setup or persisted session in the
modal rather than creating or copying a conversation. Blank setup tabs are
typed client-only placeholders with workspace-and-kind-scoped IDs; persisted tabs map to existing
task sessions. Setup IDs never enter backend hydration and prevent blank ordinary/config tabs from
aliasing each other while retaining client-side cleanup for abandoned setup.

The backend remains authoritative for capability classification. Boot restoration derives
`kind="config"` only from `task.metadata.config_mode == true`; it never infers privileges from a
title or frontend input. The dedicated `/config-chat` endpoint remains responsible for config-mode
MCP preparation, while `/quick-chat` retains ordinary and repository-backed behavior. A missing
`kind` in an older frontend/boot shape normalizes to `chat`.

Boot restoration lists workflow-less, non-automation ephemeral tasks for the active workspace,
hydrates their primary task sessions, assigns their kind, and sorts them by last activity. The
modal and active tab remain closed/unset at boot. Workspace filtering is enforced at restoration,
provider selection, launch, and activation boundaries.

Real tab closure uses the existing Quick Chat confirmation and task deletion lifecycle for both
kinds. Closing the modal or switching tabs is presentation-only. Configuration tasks remain
excluded from the seven-day ordinary-chat sweeper, so explicit tab/workspace deletion is their
cleanup owner.

Inline clarifications stay inside `QuickChatContent` with a bounded scroll region, touch-capable
resize handling, and a collapse/expand affordance. This preserves message context without a nested
dialog on desktop or mobile.

## Consequences

Configuration conversations become durable and readable without a schema migration or a second
chat implementation. Ordinary and configuration sessions can coexist while preserving distinct
setup and backend capability contracts. The floating Settings panel owns only presentation state;
shared session state, pending initial prompts, messages, and task identity must remain transferable.
Tab and hydration code must carry an explicit kind, and workspace transitions must filter rather
than assume all hydrated sessions share the current workspace.

Config sessions have no automatic idle cleanup. Any creation, delete, or workspace-removal path
must continue to use task lifecycle services so config-mode tasks cannot become hidden orphans.
The separate config-chat tab state is removed. The Settings provider owns the floating panel's open
and selected-view state while delegating durable sessions and task cleanup to the shared model.

## Alternatives Considered

- **Persist a separate configuration-popover tab model.** Rejected because it creates competing
  stores and duplicates restoration, prompt delivery, and task lifecycle behavior. The accepted
  floating panel is only a presentation over the unified typed-session store.
- **Infer configuration sessions from task titles.** Rejected because titles are mutable display
  data and must not control elevated backend capabilities.
- **Add a dedicated configuration-tab table.** Rejected because existing task metadata, primary
  sessions, and boot hydration contain the required durable state.
- **Include configuration tasks in the seven-day quick-chat sweeper.** Rejected because config
  sessions intentionally have explicit lifecycle semantics and may be durable administrative
  conversations.
- **Render clarification in a nested modal.** Rejected because it obscures the conversation context
  needed to answer the question.
