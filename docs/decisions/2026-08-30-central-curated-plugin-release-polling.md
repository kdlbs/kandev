# ADR-2026-08-30-central-curated-plugin-release-polling: Poll Curated Plugin Releases Centrally

**Status:** accepted
**Date:** 2026-08-30
**Area:** workflow, security

## Context

The official marketplace index is rebuilt from the central `plugin-registry/plugins.yaml` allowlist,
but plugin releases do not currently trigger that workflow. Giving every third-party plugin
repository authority to dispatch a Kandev deployment would spread credentials and a replay surface
outside the curation boundary. GitHub's native schedule is centrally owned, although scheduled
runs remain best-effort. Frequent polling also consumes release API requests for every curated repo.

## Decision

Kandev polls only the repositories named in the checked-out curated allowlist every three hours.
The poll uses a read-only repository token and conditionally calls the existing Pages workflow;
Pages write and OIDC permissions remain confined to that called workflow. A release is publishable
only after exact package-name, manifest identity/version, and package-integrity verification.

Publication targets a four-hour SLO under normal GitHub Actions scheduling, not a deterministic
wall-clock guarantee. The daily 06:00 UTC rebuild remains the recovery path and star refresh.
Existing curated entries retain their last known-good record when one publisher has a bad release;
delisted repositories are never restored from prior output.

## Consequences

Third-party plugin repositories hold no Kandev credential and cannot select a repository to build or
deploy. New releases normally reach the official index after the next poll, while provider
delay or failure remains visible in Actions and may defer publication until a later poll or the daily
fallback. The central repository performs additional release API requests proportional to the
allowlist size and owns package verification before publication. The three-hour schedule reduces
these routine requests while still publishing releases faster than the daily fallback.

## Alternatives Considered

- A five-minute poll would find releases sooner but make 36 times as many routine release API
  requests for each curated repository. Plugin releases do not require that frequency.
- Plugin-origin `repository_dispatch` would usually signal faster, but it would distribute Kandev
  authority across third-party repositories and require every curated publisher to integrate it.
- A GitHub App webhook receiver or managed external scheduler could offer a stronger latency bound,
  but adds hosted infrastructure, app installations, secrets, signature handling, and monitoring.
  Revisit this only if a deterministic publication guarantee becomes a requirement.
