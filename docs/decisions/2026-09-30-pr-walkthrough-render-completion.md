# ADR-2026-09-30-pr-walkthrough-render-completion: Complete walkthrough generation after a verified render

**Status:** accepted
**Date:** 2026-09-30
**Area:** workflow, infra

## Context

PR #4068 exceeded the generation job's 15-minute limit. OpenCode repaired several invalid drafts and rendered the outputs twice. It remained running until GitHub cancelled the job. Publication never started, although the diagnostic artifact contained rendered files.

The current adapter requires a zero process exit and two non-empty files. This rule makes completion depend on another model response after rendering. Non-empty files alone cannot establish that an output pair belongs to the current attempt.

## Decision

The trusted renderer records successful completion after it commits both final files. The receipt binds their hashes to the exact event head and PR number. The agent cannot edit this receipt or either final file directly.

The workflow adapter stops its owned agent process group after it observes the receipt. It then validates the receipt, trusted identity, output hashes, schema, and rendered HTML. This validation determines generation success. An expected process termination after rendering does not require a final model response.

Unexpected non-zero exits, external cancellation, missing receipts, and invalid output pairs remain failures. The adapter never converts GitHub cancellation into publication success.

One total generation deadline covers all attempts. An incomplete zero-exit attempt can retry once within the remaining budget. Cleanup and diagnostics finish before the outer job deadline.

This decision extends the [filesystem runner contract](2026-08-22-pr-walkthrough-filesystem-runner.md). The agent still writes the draft and invokes the fixed renderer. The workflow validates agent-built outputs rather than creating missing outputs after an agent exits.

## Consequences

- Successful rendering no longer depends on an additional model response.
- Failed and stale output pairs cannot pass through a non-empty-file check.
- Runner adapters need process cleanup and a deterministic verification step.
- The receipt is a completion record, not a security signature. Existing tool permissions protect its write boundary.
- Model changes retain the same renderer and completion contract.

## Alternatives Considered

- **Increase only the job timeout:** This gives the agent more time but leaves completion dependent on its final response.
- **Accept any files after cancellation:** This can publish stale, incomplete, or mismatched outputs and ignores cancellation intent.
- **Trust a completion phrase:** A model can emit the phrase without completing the renderer contract.
- **Render only after process exit:** The agent loses direct feedback for repairing invalid data.
