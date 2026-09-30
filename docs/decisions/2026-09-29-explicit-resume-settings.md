# ADR-2026-09-29-explicit-resume-settings: Explicit recovery may omit mode and model overrides

**Status:** accepted
**Date:** 2026-09-29
**Area:** protocol

## Context

The user wants an Auggie task to fail when its selected model or mode cannot be
applied, then permits the Resume recovery action to restore the conversation
without those selections. Generic model fallback and synchronous mode-RPC
acknowledgment are not evidence of this recovery intent.

## Decision

For Auggie task sessions, enforce configured settings on ordinary start/resume.
An explicit Resume after failure may omit both model and mode overrides for one
attempt, retaining provider-restored values and the same conversation identity.
Expose that consequence beside the action. Keep saved profile and session
selections intact and keep ACP confirmation strict. Other providers and Office
retain their existing launch policy.

This is an explicit exception to mode reapplication in
[session permissions](2026-09-27-session-permissions-without-settings-mutation.md),
not permission to claim an unconfirmed mode was applied or to mutate settings.
The [owning requirement](../specs/agents/requirements/explicit-resume-settings.md)
and [design](../specs/agents/system-design/explicit-resume-settings.md) define
eligibility and attempt ownership.

## Consequences

Recovery can use a different mode/model from the saved selection, including a
more permissive provider-restored mode. The action explains this before dispatch,
and a durable notice records successful omission. Future ordinary resumes still
validate saved settings. Recovery cannot guarantee a provider session is valid.

## Alternatives Considered

- Automatic fallback after any failure: rejected because the user wants the
  initial failure and an explicit recovery decision.
- Treat RPC success without a mode report as confirmation: rejected because it
  changes a shared permission guarantee and does not solve model rejection.
- Erase saved selections: rejected because a temporary recovery must not alter
  profile configuration or other sessions.
