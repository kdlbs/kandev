# ADR-2026-07-20-provider-neutral-remote-repositories: Provider-Neutral Remote Repositories

**Status:** accepted
**Date:** 2026-07-20
**Area:** backend, frontend, protocol

## Context

Task creation accepted a `github_url` and reconstructed clone URLs from a small provider enum. That prevented configured GitLab and Azure DevOps repositories from using the same picker and lost provider-specific canonical URLs. Private Azure Repos also need authentication during backend materialization without exposing the workspace PAT to a task runtime.

## Decision

Task repository inputs accept additive `remote_url` and provider identity fields while retaining `github_url` compatibility. Provider-backed repository rows persist a canonical, credential-free remote URL, and clone resolution prefers that stored value.

Integration credentials may authenticate a backend-owned clone or fetch through a provider-specific child-process mechanism. They must not be written to persisted URLs, task metadata, command arguments, structured logs, or agent environment variables. Push credentials and credentials needed inside an executor remain a separate runtime concern.

## Consequences

Configured source-control providers can share repository discovery, branch selection, task creation, and persistence contracts. Provider URL validation and credential resolution stay backend-owned. Adding another provider requires URL normalization, discovery/branch adapters, and an explicit materialization-auth policy.

The legacy `github_url` field remains until existing API, WebSocket, and MCP callers migrate. Azure's workspace PAT can support the initial managed checkout but does not automatically authorize task commands or pushes.

## Alternatives Considered

- Keep translating every remote into `github_url`: rejected because the name and parser encode the wrong provider contract.
- Put PATs in clone URLs or task environment variables: rejected because URLs persist and agent environments broaden secret exposure.
- Require users to configure Git credentials before any Azure clone: rejected because the configured read-only PAT already provides a narrower backend-only materialization path.
