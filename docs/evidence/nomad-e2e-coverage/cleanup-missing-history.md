# CR-007: missing pinned-job history retains unknown runtime

Recorded 2026-10-28 (Asia/Singapore), based on PR head `9b006aea5f`.

## Failure and correction

A pinned job can commit submission without the backend checkpointing its allocation. After Nomad garbage-collects job/allocation history, neither a missing job nor an empty allocation list proves that its processes stopped. The former `pinnedNeverAllocated` shortcut fabricated an empty drain receipt and released inventory in that unknown state.

Removed the history-based pinned shortcut, including receipt-resume and inventory-finish exemptions. A pinned operation without positive runtime evidence now retains inventory and attempts quarantine. The durable `RefusedBeforeStage` marker still permits no-runtime settlement, preserving the original job-stop behavior where a job is readable. Frozen requirements/design are unchanged.

Permanent regression `TestNomadPinnedMissingHistoryRetainsUnknownRuntime` validates each fixture before cleanup and covers registered/unknown-registration states through initial cleanup, pending resume and an existing empty receipt. All six initially failed with a successful cleanup, one inventory release and zero quarantine calls. They now require retention, no absent checkpoint, no inventory release and a quarantine attempt; fresh cleanup cannot mint a drain receipt. Prior tests that treated missing history as proof now assert retention, while proven pre-stage refusal retains its successful cleanup checks.

## Local verification

- Focused cleanup regression and pinned cleanup/resume tests: pass.
- Race checks: Nomad, launchwrapper, agentctl process and utility selections pass. Lifecycle selection `Nomad|Pool|LocalWorktree|Reconcile` passes with the externally inherited `OTEL_EXPORTER_OTLP_ENDPOINT` unset. The package's existing goleak guidance requires no ambient endpoint; an initial run's assertions passed but its ambient batch exporter leaked. No assertion, leak check or skip was changed.
- All 600 offline Nomad runner tests pass, including real localhost TLS transport tests.
- TLS fixture generation now explicitly requests named-curve EC parameters and SHA-256 for CA/CSR/server signing. macOS LibreSSL defaults caused Python certificate verification to reject explicit EC parameters, then weak signature digests. Failing-first command-shape assertions and the existing real transport tests cover the fix; certificate verification remains enabled.
- Affected-package vet passes. Normal changed-package lint and commit hooks are required for delivery.
- Broader local build attempts hit ENOSPC during linking. Those attempts are not passing builds; an isolated Tank build is a separate next step. Full backend suite is not rerun; the earlier baseline-comparison evidence applies only to its recorded candidate.

## Rollout boundary

The owner subsequently authorized build/deployment and real Tank/Neo E2E after local verification. No deployment or real-worker result is established by this document. Keep #4835 draft. Review of the correction, matching runtime builds, required nono/client/credential/network integration and the approved rollout path remain prerequisites; old live-proof exclusions remain open until receipts show otherwise.
