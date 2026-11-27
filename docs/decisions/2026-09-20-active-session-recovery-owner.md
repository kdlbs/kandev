# ADR-2026-09-20-active-session-recovery-owner: Active session recovery ownership

**Status:** accepted
**Date:** 2026-09-20
**Area:** frontend

## Context

The user inspected failed startup, interrupted-session, runtime-installation and
provider-quota fixtures. Distinct renderers have different layouts and can mount
recovery actions in both the transcript and the stopped composer. The user
approved one shared recovery card in place of the blocked composer, with compact
historical transcript entries. Recovery eligibility differs across these causes.

## Decision

The mounted session's composer region owns active recovery when messaging is
blocked by an unresolved failure. Cause-specific adapters share one card and
retain their existing operation permissions. Transcript rows retain the error's
original cause, timestamp and safe details, without competing mutation controls.
A standalone transcript without a composer can own the same card as a fallback.
A usable session keeps its composer; historical errors cannot disable it.
All eligible recovery actions are directly visible as individual buttons, with
the recommended action first and standard app button styling. Desktop rows wrap and phone buttons
stack; no More options menu is required. Existing confirmations remain intact.

This qualifies the active-control placement in
[error scope and conversation history](2026-09-14-error-scope-and-history.md).
Its chronological persistence, normal transcript scroll ownership, independent
task-error scope and correlation rules remain in force. No new persistence,
backend recovery operation or provider policy is introduced.

## Consequences

Users recover where they would otherwise send a message. Cause distinctions
remain visible within consistent styling and action order. Drafts and attachments
must survive the editor's replacement. The active owner cannot depend on whether
the history page containing the failure is loaded. Expanded details need a bounded
recovery-region scroll area so they cannot consume a phone's whole viewport.

A quota failure with no eligible recovery operation shows its prerequisite;
it does not gain an invented model switch or retry. Archive and Delete stay in
task menus. Independent workspace failures retain their own scope, while explicitly
correlated dependent panes link to the session owner.

## Alternatives Considered

- Transcript-only actions preserve one scroll region but can leave recovery far
  from a disabled composer and make active control availability depend on paging.
- Duplicate transcript and composer actions are discoverable but create competing
  controls, pending states and explanations for one failure.
- Separate runtime/quota/error cards preserve specialized controls but allow
  layout and hierarchy to diverge. Shared presentation with typed capabilities
  preserves the same semantics without that divergence.
