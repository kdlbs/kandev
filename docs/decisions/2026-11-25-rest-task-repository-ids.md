# ADR-2026-11-25-rest-task-repository-ids: Accept repository IDs on REST task creation

**Status:** proposed
**Date:** 2026-11-25
**Area:** backend

## Context

`POST /api/v1/tasks` decodes only `repositories`. A request that selects repositories through `repository_ids` can succeed with zero attachments.
Repository set creation already accepts the ID list as a compatibility field.

## Decision

Accept `repository_ids` as an alias for ID-only `repositories` entries at task creation.
Normalize the alias at the HTTP boundary. Reject requests with two arrays because either array can express a different selection.

## Consequences

ID-only clients can use the same request shape as repository set clients.
Clients that specify a base branch, checkout branch, or another per-repository option use the canonical `repositories` field.

## Alternative considered

Reject `repository_ids` with HTTP 400. An ID-only caller must then change its request to attach a repository.
