# ADR-2026-07-20-managed-github-app-registration: Manage Self-Hosted GitHub App Registration at Runtime

**Status:** superseded by ADR-2026-07-21-workspace-selectable-github-app-registrations
**Date:** 2026-07-20
**Area:** backend, frontend, security

## Context

This unpublished decision introduced runtime GitHub App onboarding around a singleton deployment
registration. It was superseded before deployment when product ownership moved to explicit
per-workspace selection from a multi-registration catalog.

## Decision

No part of this decision is an active architecture contract. Runtime manifest onboarding and
encrypted credential bundles continue only through
[ADR-2026-07-21-workspace-selectable-github-app-registrations](2026-07-21-workspace-selectable-github-app-registrations.md).
The singleton and configuration-backed registration sources described by the original draft never
shipped and are removed rather than migrated.

## Consequences

- This file is a supersession marker; implementers must follow the replacement ADR and current
  workspace GitHub authentication spec.
- There is no singleton migration or compatibility promise for the unpublished implementation.

## Alternatives Considered

See the replacement ADR for the accepted catalog model and rejected singleton alternatives.
