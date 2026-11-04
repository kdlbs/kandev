---
id: "01-model-fallback"
title: "Add dynamic model fallback"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-PR-WALKTHROUGH-001
acceptance_criteria:
  - AC-UI-PR-WALKTHROUGH-001.7
  - AC-UI-PR-WALKTHROUGH-001.13
  - AC-UI-PR-WALKTHROUGH-001.14
  - AC-UI-PR-WALKTHROUGH-001.15
  - AC-UI-PR-WALKTHROUGH-001.16
  - AC-UI-PR-WALKTHROUGH-001.17
  - AC-UI-PR-WALKTHROUGH-001.18
system_design:
  - ../../specs/ui/system-design/pr-walkthrough.md
---

# Task 01: Add dynamic model fallback

## Summary

Make the trusted walkthrough supervisor discover and try current eligible free
`opencode` models, then use the requested Go model when free candidates are
absent or exhausted. Preserve the existing receipt, verification, cleanup,
deadline, cancellation, and best-effort publication gates.

## In scope

- Parse refreshed verbose metadata from the pinned CLI, restrict free rotation
  to the `opencode` provider, validate explicit zero prices, text and tool
  capabilities, and order unique candidates deterministically. Retain other
  provider metadata only for the explicit Go fallback.
- Detect attributable quota, rate-limit, and unavailable-model errors even when
  the CLI remains live. Stop and reap its group before clearing state and
  trying the next model. Keep separate diagnostics and model identities.
- Wire Go authentication from `OPENCODE_API_KEY`, the requested paid model,
  shared time budget, and model-compatible options into the trusted workflow.
- Add focused regression coverage and update workflow contract tests and
  workflow CI registration if new test files are introduced.

## Out of scope

- New model-provider choices, renderer or publication changes, and PR review.
- Exposing credentials or treating an unknown error as a quota error.

## Acceptance

- Refreshed catalog additions and removals change the free candidate list
  within the `opencode` provider; malformed, paid, non-text, and non-tool models
  are excluded, and models from other providers cannot enter free rotation.
  Empty catalog and exhausted free candidates reach the Go model, while failed
  discovery does not silently trigger paid use.
- Recognized model quota, rate-limit, or availability errors cause clean model
  rotation, including when the process hangs. Tests distinguish unrelated
  title-agent errors and untrusted content from the selected model's errors.
- Completion, invalid receipts, cancellation, cleanup failures, and the total
  deadline retain their existing safety rules; per-attempt diagnostics report
  model and outcome without secrets.

## Verification

```bash
python3 .github/scripts/pr-walkthrough-runner_test.py
python3 .github/scripts/pr-walkthrough-workflow-contract_test.py
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `.github/scripts/pr-walkthrough-runner.py`
- `.github/scripts/pr-walkthrough-runner_test.py`
- `.github/scripts/pr-walkthrough-workflow-contract_test.py`
- `.github/workflows/pr-walkthrough.yml`
- `.github/workflows/lint-action-pinning.yml` if registration changes
- This plan and work order for status and results

## Dependencies

None. Confirm the pinned OpenCode CLI's catalog and Go credential behavior
before relying on either interface; record any unavailable live credentials.

## Risks

CLI error output and catalog format may change. Free quotas may be shared.
Selected models can retain or train on PR context. The paid Go credential may
be expired, lack a subscription, or be restricted by region.

## Parallelism

Sequential.

## Inputs

- PR walkthrough requirements and system design, especially model fallback.
- ADR 2026-10-28 and prior completion and advisory generation decisions.
- Existing supervisor and workflow contract tests.

## Results

Implemented dynamic model discovery and fallback in the trusted runner.

- RED: the pre-change runner has no `discover_models`, `build_candidates`, or
  `classify_provider_error`, and its `RunConfig` exposes only the fixed-attempt
  fields. The rotation regression therefore cannot pass against the fixed-model
  implementation.
- GREEN: the runner refreshes the pinned CLI catalog, restricts free rotation
  to the `opencode` provider, selects explicit zero input/output price models
  with text input/output and tool calling, deduplicates and orders them
  deterministically with the existing DeepSeek free model first, and keeps the
  requested Go Contributor model last. Other providers, including free
  `opencode-go` models, cannot enter free rotation; their metadata remains
  available only for the explicit Go fallback. Invalid identifier lines and
  conflicting duplicate records are excluded instead of corrupting another
  model. It adds the Go candidate only when `OPENCODE_API_KEY` is configured.
- Rotation: a live process that reports a structured, attributable quota,
  rate-limit, or model unavailability error is stopped and reaped before the
  next candidate starts. A final unterminated stderr record is still
  classified. Title-generation errors, errors from another model, unrelated
  error text, and patch text do not rotate. The variant is passed only when
  the selected model advertises it.
- Safety: cancellation, deadline, cleanup, invalid receipts, verification
  failures, and unrecognized exits remain terminal. Discovery failure fails
  closed without selecting the paid model. Each attempt records its model,
  stop reason, provider-error category, and verification result without
  credentials.
- Verification: 45 runner tests and 31 workflow contract tests pass. Document
  validation, specification lint, and `git diff --check` pass. Local probes of
  the pinned CLI v1.19.3 confirmed the catalog shape, 9 eligible `opencode` free
  models, Go provider metadata, and `OPENCODE_API_KEY` routing (dummy key
  reached the provider and failed with invalid credentials, not a
  missing-provider error). A local run with a mock provider also produced the
  real main-generation and title-generation stream-error shapes. The title
  error used the same model with `small=true` and `agent=title`; the classifier
  ignored it and matched only the main attempt.
- Remaining external evidence: a later eligible Actions run must confirm the
  Actions secret, shared free quota behavior, Go subscription access, and
  public delivery.
