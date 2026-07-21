# ADR-2026-07-20-repository-provider-origin-identity: Persist Provider Origin In Repository Identity

**Status:** accepted
**Date:** 2026-07-20
**Area:** backend, frontend

## Context

Repository identity previously stored a provider plus owner and repository name.
That is sufficient for GitHub's single public origin, but it is ambiguous for
GitLab and other self-managed providers where the same project path can exist on
multiple hosts. Provider-backed repositories may also have no local checkout,
so reading `remote.origin.url` at action time is not a reliable identity source.

## Decision

Persist a normalized `provider_host` with each provider-backed repository and
carry it through repository models, DTOs, create/update requests, and task
repository selection. Provider actions and external-resource associations must
match provider, host, and full project path. Local repository discovery may
derive the host from the configured origin, but callers fail closed when a
host-sensitive provider identity is missing or mismatched.

Existing rows migrate with an empty host because their origin cannot be inferred
reliably from database metadata alone. They become eligible for host-sensitive
actions only after local-origin discovery or an explicit provider import/update
records the host.

## Consequences

Self-managed provider repositories are unambiguous across workspaces and host
changes, and frontend selection can use the same identity contract as backend
authorization. Repository persistence and API contracts gain one field, and
legacy provider rows with unknown hosts require refresh or re-import before
GitLab-specific linking and creation actions are available.

## Alternatives Considered

- Read `.git/config` for every action. Rejected because remote/provider
  executors and newly imported repositories may not have a local checkout.
- Combine the active workspace connection host with owner/name at request time.
  Rejected because changing the connection could silently reinterpret an
  existing repository as belonging to a different GitLab instance.
- Keep host only in frontend state. Rejected because backend association and
  execution boundaries must enforce the same identity independently.
