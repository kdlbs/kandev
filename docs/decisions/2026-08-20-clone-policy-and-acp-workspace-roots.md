# ADR-2026-08-20-clone-policy-and-acp-workspace-roots: Attest Clone Policies and Negotiate ACP Roots

**Status:** accepted
**Date:** 2026-08-20
**Area:** backend, protocol, security

## Context

Task Git metadata is derived from a checkout, but Docker, SSH, and Sprites create their checkout after the host lifecycle has intentionally discarded host paths. Separately, agentctl tracks canonical workspace source roots for its own file services but did not pass that narrow source set to ACP session creation.

## Decision

Clone-based launch requests carry an intent-only requirement rather than host filesystem metadata. Each executor resolves and validates its own canonical regular checkout. Immediately before configuring or starting the agent, agentctl batch-attests every canonical primary and secondary task checkout; lifecycle renders policy only from that returned checkout/Git-directory set. Failure is closed and sanitized.

Lifecycle passes only materialized task checkouts and durable task-owned attachments to agentctl; source repositories used to create a worktree are never ACP roots. Agentctl revalidates that exact server-owned set before ACP capability negotiation, and includes it in `session/new` only when the initialized provider advertises `sessionCapabilities.additionalDirectories`.

## Consequences

Host paths cannot become remote policy inputs or error text. A root that is removed, swapped, foreign, or otherwise fails final attestation cannot reach policy rendering, `ConfigureAgent`, `Start`, or ACP session creation. Providers without the required capability fail with `git_metadata_projection_unsupported`; Kandev neither widens their access nor fabricates a capability. Executor integrations must keep clone attestation, cleanup, resume, and rebind behavior under regression coverage.

## Alternatives Considered

Passing host projections to clone runtimes was rejected because paths are not authoritative on the executor and disclose host layout. Sending the task-root parent or always sending `additionalDirectories` was rejected because both can widen scope silently on providers that did not negotiate support.
