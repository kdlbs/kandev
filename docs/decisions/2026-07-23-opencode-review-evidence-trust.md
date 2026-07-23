# ADR-2026-07-23-opencode-review-evidence-trust: Trusted OpenCode Review Evidence

**Status:** accepted
**Date:** 2026-07-23
**Area:** workflow, infra

## Context

Routine PR review can use OpenCode running inside a trusted base-commit wrapper, but GitHub records `github-actions[bot]` for every shared workflow identity. That identity is forgeable by unrelated Actions workflows, and incremental review cannot prove full-PR coverage. Fork-safe workflows also need review evidence without granting untrusted PR code write authority.

## Decision

Trusted jobs run only from the base-controlled `pull_request_target` workflow, check out and assert the immutable head SHA, use only `BASE_SHA...HEAD_SHA`, and load the publisher from `BASE_SHA`; no App credential reaches the model step. Only same-repository review attaches the protected `opencode-review-trusted` environment and mints the dedicated current-repository-only installed GitHub App pull-request-write token; it uses `OPENCODE_REVIEW_ENV_APP_PRIVATE_KEY`, created only in that environment with no repository/org fallback secret, and administrators restrict environment deployment branches to the protected default branch. Its login is `${OPENCODE_REVIEW_APP_SLUG}[bot]`. Fork review never attaches this environment, remains advisory under the base-controlled workflow's GitHub token, and can never qualify.

The wrapper accepts exactly one findings block covering the whole trimmed model output (an optional JSON fence is allowed inside it). Valid empty output emits the wrapper clean marker in an exact-head `COMMENT` review; valid findings emit an action-required terminal review. Every malformed, missing, or failed model result attempts an exact-head non-clean diagnostic review and fails the job. A same-head non-clean review remains blocking even if a later clean review appears; only a new head can recover. Historical no-findings issue comments are non-actionable, while diagnostic and fallback comments remain actionable.

Every trusted-wrapper terminal review (clean, blocked, or diagnostic) includes a machine marker containing its exact `GITHUB_RUN_ID` and `GITHUB_RUN_ATTEMPT`; the clean marker remains a separate wrapper-owned semantic marker. `scripts/pr-state` grants `trusted_default_producer` only to an exact-head clean COMMENTED review whose author exactly equals the explicit configured App login, whose marker matches the authenticated latest workflow run and attempt, and whose PR is explicitly known same-repository. It queries only `actions/workflows/opencode-code-review.yml/runs`, requires the base-controlled workflow path, `pull_request_target` event, PR/head association, and selects the latest `(run_number, run_attempt, id)` generation. It then requires the exact attempt's `opencode-review-same-repo` job plus run conclusion to be completed/success, and performs its authoritative final head read only after those Actions API calls. Display names and check rollups never authenticate evidence. The canonical emitted `trusted_producer` is `true` only when the raw marker/App predicate and successful authenticated workflow/job predicate both pass, `false` for known failure, and `unknown` for incomplete/API state. Missing or failed API reads, marker mismatch, a post-API head change, and pending, cancelled, skipped, neutral, or failed latest attempts invalidate old clean evidence. This shortcut is limited to routine same-repository work with successful full-PR evidence and all existing check, thread, and blocker gates.

External setup is required: `OPENCODE_REVIEW_ENABLED=true`, App client ID, App slug, the protected environment-only private-key secret, current-repository-only installation, and Pull requests write permission. Missing or misconfigured environment protection remains fail-closed. This repository does not mutate GitHub settings.

For PR delivery, qualifying trusted evidence is the default semantic path to
avoid spending local frontier-review capacity on routine work. Local review is
exceptional: unusually large/complex/cross-cutting architecture, terminal
readiness that cannot wait for contradictory or repeatedly unavailable evidence,
an explicit request, or a deep automated-review gap. Security auditing is
likewise limited to high-impact new/changed authz, workspace-isolation, secrets,
untrusted-execution, or credential-trust boundaries, explicit request, or
concrete automated security concerns.

No product spec applies because this is workflow and infrastructure evidence policy.

## Consequences

Selected OpenCode evidence can end polling without unrelated bots once CI and all blocker gates are clean, while generic Actions activity cannot satisfy the gate. COMMENT deliberately avoids representing automated review as human approval. This PR-first default reduces token cost while preserving fail-closed exact-head qualification; a small scope-preserving fix uses its finding, focused regression, final verification, and fresh exact-head review rather than restarting every local gate. The App introduces credentials, installation, and operational setup; unavailable setup uses the exceptional local-review route when terminal readiness requires it.

## Alternatives Considered

- **Claude or Greptile as the routine producer:** useful independent reviewers, but their availability, identity, and delivery paths do not provide this trusted OpenCode-wrapper contract.
- **Local-only frontier review:** rejected as the routine path because it consumes local review capacity and tokens; retained as the exceptional fallback.
- **Shared Actions identity:** rejected because any Actions workflow can impersonate `github-actions[bot]`.
- **Check-correlation-only evidence:** rejected as complex and incomplete without a dedicated review identity and terminal semantic review.
- **Dedicated GitHub App:** chosen despite credentials, installation, and operational ownership because it provides a non-forgeable producer identity.
