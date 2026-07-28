# ADR-2026-07-26-user-managed-agent-runtime-updates: User-Managed Agent Runtime Updates

**Status:** accepted
**Date:** 2026-07-26
**Area:** backend, frontend, protocol

## Context

Kandev launches core ACP agents from npm-distributed runtimes. Exact
repository pins make launches reproducible, but delay upstream models and
capabilities until Kandev reviews, merges, and deploys a dependency update.
Unconditionally resolving `latest` on every launch removes that delay but adds
registry work and makes change timing invisible to the operator.

npm provides an execution cache that can satisfy unversioned package
invocations, but documents it as a best-effort cache rather than durable
installation state. ACP separately exposes a negotiated protocol version and
runtime-advertised capabilities during initialization.

## Decision

Kandev does not maintain exact package-version pins for its managed npm agent
runtimes. Normal launches identify the package without a version or an
explicit `latest` tag and may reuse npm's execution cache.

The install operator owns runtime advancement through a Settings action.
Kandev resolves the current and upstream target versions, executes only the
built-in package update recipe, streams progress, and re-probes the host
runtime after success. Update actions affect future probes and launches; they
never replace an active session.

ACP protocol negotiation, normalized capability discovery, and the existing
per-agent compatibility dialects are the compatibility boundary. Kandev
accepts the risk that an upstream package can contain a behavioral regression
without changing its ACP protocol version. It surfaces initialization or probe
failures and does not silently roll back to a repository-pinned runtime.

The first managed set is Claude, Codex, OpenCode, Copilot, and Gemini. The
Settings action targets the Kandev host runtime only. Remote executors and
containers use their own unversioned runtime resolution when they launch.
Native-only distribution channels and separately distributed passthrough or
authentication helper packages remain outside this boundary.

## Consequences

Operators can opt into newly released models without waiting for a Kandev
release, while ordinary launches can benefit from npm cache reuse. The UI can
attribute an update to a visible current and target version and immediately
show newly advertised models and modes.

Launches are no longer reproducible by Kandev commit alone. An npm cache
eviction, executor rebuild, or fresh machine may resolve a newer runtime even
without an explicit Settings update. Incident diagnosis must therefore record
the runtime-reported agent version rather than infer it from source.

Compatibility depends on upstream ACP discipline and Kandev's dialect layer.
A package can regress while retaining the same protocol version, and the first
operator to update may encounter that regression. Rollback and version
selection are deliberately not provided in this iteration.

## Alternatives Considered

- Exact repository pins with scheduled update pull requests were rejected
  because model availability remains coupled to Kandev's review and deployment
  cadence.
- Resolving `latest` on every launch was rejected because it adds repeated
  registry work and makes runtime changes happen without an explicit operator
  action when npm cache metadata becomes stale.
- A Kandev-owned package installation directory and lockfile were rejected for
  this iteration because it recreates a package manager and durable version
  inventory beyond the requested best-effort npm cache behavior.
- Updating every configured executor from the host Settings page was rejected
  because remote credentials, platform differences, partial failure, and
  executor ownership require a separate product contract.
