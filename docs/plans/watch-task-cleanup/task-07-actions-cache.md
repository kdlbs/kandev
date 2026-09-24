---
id: "07-actions-cache"
title: "Cache Actions observations with bounded expiry"
status: done
wave: 7
depends_on: ["06-adaptive-discovery"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-002
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-002.1
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-002.2
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-002.3
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-002.4
system_design:
  - ../../specs/integrations/system-design/github-workflow-attention.md
---

# Task 07: Cache Actions observations with bounded expiry

## Summary

Cache Actions observations with bounded expiry. Implement with TDD and preserve workspace identity.

## In scope

Share workflow-run and job observations across batch enrichment and service
status/feedback reads. Scope by credential generation and repository; runs also
use head SHA, and jobs use run ID plus attempt. Classify each PR separately.

Use 30 seconds for empty, pending, running, unknown, or attention-required data.
Use 5 minutes only for nonempty completed data without attention requirements.
Keep 512 entries each for runs and jobs, and coalesce concurrent misses. Do not
cache errors or change merge/check eligibility. Preserve the existing 4-worker,
5-second batch enrichment budget.

Explicit refresh invalidates relevant outer and Actions cache entries. Prevent
in-flight old observations from repopulating after invalidation. Head and
credential changes must select fresh entries. Future mutation hooks can reuse
this invalidation seam; no approval or rerun UI is added.

Add TestWorkflowAttentionCacheExpiryAndInvalidation to a new
workflow_attention_cache_test.go. Cover same-SHA reruns, new run attempts,
empty/approval/error results, concurrent callers, credential isolation, different
PRs sharing one SHA, explicit refresh, and bounded eviction. Use fake time.
Retain the existing workflow attention classification suite.

Update `docs/public/integrations.md` during implementation to describe the
shipped freshness bounds. Keep the public guide unchanged during planning.
Run these additional checks after that documentation edit:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Out of scope

New UI controls, credential policies, global schedulers, and unrelated providers.

## Acceptance

- Meet every linked acceptance criterion with deterministic regressions.
- Preserve existing policy, scope, and error-handling contracts.
- Record the expected red test result and passing package results.

## Verification

Run from the repository root.

```bash
(cd apps/backend && go test ./internal/github -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/github/service_pr_watch_batched.go`
- `apps/backend/internal/github/service_pr.go`
- `apps/backend/internal/github/service.go`
- `apps/backend/internal/github/ttl_cache.go`
- `apps/backend/internal/github/workflow_attention.go`
- `apps/backend/internal/github/workflow_attention_cache.go`
- `apps/backend/internal/github/workflow_attention_cache_test.go`
- `apps/backend/internal/github/workflow_attention_test.go`

- `docs/public/integrations.md`

## Dependencies

06-adaptive-discovery.

## Risks

Same-SHA results can change. Never freeze a completed workflow collection indefinitely.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/github-workflow-attention.md)
- [Design](../../specs/integrations/system-design/github-workflow-attention.md)
- Source files and existing adjacent tests listed above. New helper/test files are created as needed.

## Results

Implemented bounded Actions observation caches shared by batched workflow
enrichment and service status, feedback, and watch reads. Run entries are
scoped by credential, repository, and head SHA. Job entries are scoped by
credential, repository, run ID, and attempt. Completed nonempty observations
without attention use a five-minute TTL; empty, changing, unknown, and
attention-required observations use 30 seconds. Each cache has 512 entries,
singleflight coalescing, and provider-error omission.

Explicit PR refresh invalidates the affected outer status or feedback entry
and repository Actions entries. Automatic task subscriptions, reconnects, and
retries use passive sync and preserve a completed Actions observation; only a
user refresh carries explicit refresh intent and invalidates the relevant
entries. Per-key epochs prevent an older in-flight read from repopulating an
invalidated cache. Credential changes clear the caches and credential
generations remain part of every workspace cache key. Existing workflow
classification, fork identity, check state, merge eligibility, and the
four-worker, five-second batch budget remain unchanged.

Added `TestWorkflowAttentionCacheExpiryAndInvalidation` and an in-flight
invalidation regression covering same-SHA reruns, attempts, empty and
attention data, provider errors, concurrent callers, credential isolation,
shared SHAs, explicit refresh, and bounded eviction. The focused test failed
before the cache seams existed as expected. Verification passed:

```bash
(cd apps/backend && go test ./internal/github -count=1)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```
