---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-GIT-CAPTURE-RECOVERY-001
created: 2026-10-05
owners:
  - kandev
---

# Workspace Git capture recovery system design

## Purpose and boundaries

This design extends [Git status](workspace-git-status.md) at the basic observation boundary.
It preserves publication ordering, tracker lifetime, subprocess admission, and enrichment validation.
It does not change the existing requirement that launch baseline capture waits for enriched values.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-PLATFORM-GIT-CAPTURE-RECOVERY-001 | Corrective capture; Comparison reference resolution; Consumers and errors; Validation |

## Corrective capture

`WorkspaceTracker.computeGitStatusObservation` calls `captureBasicGitStatus` inside a tracker-owned observation.
The current basic path returns `errGitStatusEvidenceChanged` immediately.
The enrichment worker already permits one corrective observation for changed detail evidence.
These are separate phases with separate existing ownership.

For a basic evidence change, retry capture once inside the same singleflight body.
Keep the existing shared context, admission class, observation ordinal, and deadline.
Each attempt owns a fresh index snapshot and its cleanup.
Dispose of the failed attempt before the corrective capture.
Do not extend the deadline or recursively start another observation.

Use `errors.Is` with the typed evidence-change sentinel.
Retry only this failure from basic capture.
A context error or tracker shutdown stops immediately.
The second attempt must validate all normal repository, comparison, index, and content evidence.
An identity change never permits mixing values between attempts.
Normal final publication still rejects an observation older than the latest accepted result.

Keep enrichment correction bounded under its existing policy.
The basic retry does not reset the enrichment correction budget or create a second worker.

## Comparison reference resolution

Each attempt reads comparison evidence twice: once while capturing and once while validating.
Each read resolves the base, ahead/behind, remote-head, and comparison references to commit object IDs.
Apply the existing reference allowlist first; an empty or unsafe reference yields an empty value without running Git.
A single remaining reference keeps the existing `rev-parse --verify` call.
Resolve two or more distinct references with one `git rev-parse <ref>^{commit}...` process.
Accept the batch only when Git exits zero and returns exactly one full SHA-1 or SHA-256 object-ID line per argument.
Allow LF or CRLF termination; extra records, including blank records, require fallback.
Otherwise resolve each reference with the existing `rev-parse --verify` path.
That fallback also covers a missing reference, a truncated result, and an echoed argument that only looks like an object ID.
A cancelled context skips the fallback and yields the same empty values the separate path yields.
The evidence comparison and its errors are unchanged.

## Consumers and errors

`GetGitStatusWithDetails` and the existing agentctl status route retain their wire shapes.
`Client.GetGitStatusWithDetails` already requests `fresh=true&details=wait`.
`Executor.captureBaseCommit` continues to consume a successful enriched result.
Completion, archive, and Review consumers retain their existing completion requirements.

Expose a narrow internal error classifier from the process package only where the API needs it.
A recovered race records debug evidence without a failed-request error.
An evidence-change failure records one warning at the request boundary.
Unexpected Git failures retain their existing severity and response classification.
The periodic tracker must use the same classification instead of treating each recoverable race as an application fault.
No extra launch-side retry loop is introduced.

## Validation

Use real temporary Git repositories with deterministic mutation barriers.
Cover a single index/worktree change, continuous changes, comparison replacement, concurrent waiters, cancellation, and shutdown.
Check cleanup of every temporary index and rejection of stale publication.

Route an HTTP status request through a real tracker and verify the final membership and detail state.
Exercise launch baseline capture with the recovered enriched result and with unavailable detail.
A successful complete enriched observation can establish the session baseline.
For backward compatibility, a successful legacy payload with every quality field omitted remains eligible; any explicit quality metadata must describe a complete ready observation.

## Related decisions

The existing subprocess and snapshot contracts remain authoritative.
See [Git subprocess execution](git-subprocess-execution.md) and [Git status publication](workspace-git-status.md#publication-and-ordering).
