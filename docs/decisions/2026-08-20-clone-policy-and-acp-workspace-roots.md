# ADR-2026-08-20-clone-policy-and-acp-workspace-roots: Attest Clone Policies and Negotiate ACP Roots

**Status:** accepted
**Date:** 2026-08-20
**Area:** backend, protocol, security

## Context

Task Git metadata is derived from a checkout, but Docker, SSH, and Sprites create their checkout after the host lifecycle has intentionally discarded host paths. Separately, agentctl tracks canonical workspace source roots for its own file services but did not pass that narrow source set to ACP session creation.

## Decision

Clone-based launch requests carry an intent-only requirement rather than host filesystem metadata. Each executor resolves and validates its own canonical regular checkout. Immediately before configuring or starting the agent, agentctl batch-attests every canonical primary and secondary task checkout; lifecycle renders policy only from that returned checkout/Git-directory set. Failure is closed and sanitized.

The lifecycle order is part of the proof: every expected canonical root must match the corresponding attested checkout and Git directory at the same index. Permutations, duplicates, missing roots, unexpected roots, and non-regular Git metadata are rejected before policy rendering.

A repository attached to a live clone runtime is not usable after a tracker-only rescan. Lifecycle materializes the complete repository batch independently in every distinct executor filesystem, then quiesces each child before asking agentctl to revalidate the full ordered task-owned root batch. Only returned approved checkout/Git-directory pairs are rendered into policy; lifecycle then configures, restarts, and restores the ACP session. Materialization and refresh are one compensating transaction: a failure removes new clones and restores every earlier executor. A prior execution returns `Ready` only after its roots, policy, and ACP session have been re-attested and restored; otherwise it remains stopped and failed with recovery guidance.

Lifecycle passes only materialized task checkouts and durable task-owned attachments to agentctl; source repositories used to create a worktree are never ACP roots. Agentctl revalidates that exact server-owned set before ACP capability negotiation, and includes it in `session/new` only when the initialized provider advertises `sessionCapabilities.additionalDirectories`.

## Consequences

Host paths cannot become remote policy inputs or error text. A root that is removed, swapped, reordered, foreign, or otherwise fails final attestation cannot reach policy rendering, `ConfigureAgent`, `Start`, or ACP session creation. A later attachment cannot remain active under stale Git grants, including when only one of several distinct executor filesystems fails. Providers without the required capability fail with `git_metadata_projection_unsupported`; Kandev neither widens their access nor fabricates a capability. Executor integrations must keep clone attestation, cleanup, resume, rebind, and later-attachment recovery behavior under regression coverage.

## Alternatives Considered

Passing host projections to clone runtimes was rejected because paths are not authoritative on the executor and disclose host layout. Sending the task-root parent or always sending `additionalDirectories` was rejected because both can widen scope silently on providers that did not negotiate support.
