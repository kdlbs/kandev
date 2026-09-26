---
created: 2026-08-28
status: done
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003
system_design:
  - ../../specs/integrations/system-design/github-pr-merge-queue.md
  - ../../specs/integrations/system-design/github-pr-merge-queue-recovery.md
legacy_specs: []
---
# Implementation Plan: GitHub Auto-Merge Reliability

## Overview

Make each automatic merge attempt safe across force-pushes, provider delays,
concurrent evaluations, and backend restarts. Bind GitHub to the reviewed head.
Reserve each attempt before the provider call. Reconcile accepted queue and
merged states, and expose one explicit retry for a failed attempt.

The provider contract lands first. The durable attempt journal follows it. The
API and shared responsive controls then expose explicit retry. Desktop and
mobile E2E tests prove the complete outcome. Public documentation closes the
delivery package.

## Scope

### In scope

- Send the reviewed head SHA with automatic GitHub merge requests.
- Pace asynchronous merge-status polling within the operation context.
- Store one durable attempt reservation and result per pull request.
- Prevent unchanged automatic retries across events and restarts.
- Reconcile active queue and merged observations with the attempt journal.
- Classify recognized legacy automatic-merge errors during migration.
- Add a scoped explicit retry command for a failed automatic attempt.
- Provide the same retry outcome in the desktop popover and phone drawer.
- Document retry and merge-queue behavior for users.

### Out of scope

- Changing the auto-merge option value or default.
- Retrying an unchanged failed attempt without a user command.
- Bypassing GitHub checks, reviews, mergeability, rules, or permissions.
- Changing the manual merge action's current head-binding contract.
- Adding a new dialog, drawer, page, or automation preference.
- GitLab merge-request automation.

## Technical approach

### Provider request and polling

Replace positional merge parameters with a request value that can carry the
merge method and expected head SHA. Require the SHA on the automatic path and
send it through both GitHub clients. Preserve the manual merge behavior.

Decode the provider's expected-head diagnostic. Wait at least one second
between pending reads. Keep the existing two-minute budget and stop waits when
the context ends. Use an injected wait boundary in focused tests.

### Durable attempt journal

Extend `github_task_ci_pr_state` with the attempt result and typed error kind.
Reserve the readiness signature and head in a transaction before GitHub is
called. Record `failed` or `accepted` after the provider result. Treat the
existing per-PR singleflight as an optimization, not the correctness boundary.

Adopt an observed active queue entry and reconcile a merged pull request as
accepted. Clear only a typed `auto_merge` error. On restart, reconcile an
`in_flight` attempt before it expires to a failed state. Do not submit the same
signature automatically.

### Explicit retry and responsive controls

Add a task-scoped endpoint that accepts the exact linked repository and pull
request. Persist retry authorization, publish the normal evaluation event, and
return `202 Accepted`. The evaluator refreshes state and runs every readiness
gate before it can call GitHub.

The shared automation controls call this command only for an `auto_merge`
error. Other stored errors show Refresh. State-loading failures keep their
existing refresh retry. Mobile continues to use the inset status drawer with
one scroll owner and a shared target of at least 44 by 44 CSS pixels.

### Integrated test support and documentation

Extend the mock GitHub controller and the E2E API client with pending, failure,
queue, merge, and head-mismatch outcomes. Prove one provider request per
authorization on desktop and mobile. Update the public GitHub integration and
session-review guides with the user-visible retry rules.

## Tests

- `AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.1`: provider request and service
  tests prove expected-head transmission and mismatch failure.
- `AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.2`: store and evaluator tests prove
  reservation before the side effect, concurrent deduplication, and restart
  safety.
- `AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.3`: signature tests prove changed
  head or gates can rearm one attempt only after every gate passes.
- `AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.4`: controller, service, evaluator,
  and E2E tests prove exact-PR retry without gate or permission bypass.
- `AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.5`: sync and evaluator tests prove
  queue and merged reconciliation with typed error clearing.
- `AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.6`: store-failure tests prove that
  GitHub is not called and a retryable per-PR error appears.
- `AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.7`: deterministic wait tests prove
  one-second pacing, cancellation, and the bounded deadline.
- `AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.8`: migration tests prove narrow
  legacy classification and preservation of unknown errors.
- `AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.9`: component and desktop/mobile
  E2E tests prove explicit merge retry and separate state refresh.
- `AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003.1-.8`: evaluator
  regression tests preserve queue adoption, same-head blocking, and new-head
  requeue behavior.

## E2E tests

- Desktop: extend `apps/web/e2e/tests/pr/ci-automation-options.spec.ts`. Produce
  an automatic merge failure, select Retry, and assert one request for the
  named pull request. Then publish an active queue entry and assert that the
  obsolete automatic-merge error clears.
- Mobile: extend
  `apps/web/e2e/tests/pr/mobile-ci-automation-options.spec.ts` with the same
  outcome under touch input. Assert the 44-pixel target, internal drawer
  scrolling, and no document-level horizontal overflow.
- Both flows cover a provider head mismatch, an unchanged failed signature, an
  accepted queue entry, a merged pull request, and a non-merge loading error.

## Work orders

- [x] [Task 01: Bind and pace asynchronous merge](task-01-bind-and-pace-asynchronous-merge.md)
- [x] [Task 02: Journal automatic merge attempts](task-02-journal-automatic-merge-attempts.md)
- [x] [Task 03: Expose explicit merge retry](task-03-expose-explicit-merge-retry.md)
- [x] [Task 04: Prove responsive merge retry](task-04-prove-responsive-merge-retry.md)
- [x] [Task 05: Document merge retry behavior](task-05-document-merge-retry-behavior.md)

## Verification commands

```bash
make -C apps/backend test
make -C apps/backend lint
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check
cd apps/web && pnpm e2e:raw --project=chromium e2e/tests/pr/ci-automation-options.spec.ts
cd apps/web && pnpm e2e:raw --project=mobile-chrome e2e/tests/pr/mobile-ci-automation-options.spec.ts
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Risks

- The GitHub client interface has many mocks. A request-value migration can
  expose callers outside automation.
- Timer tests can become slow or flaky without an injected wait boundary.
- Legacy error classification can erase the wrong error if it accepts broad
  text. The migration must recognize only stable server prefixes.
- A crashed `in_flight` attempt can require user retry when GitHub exposes no
  queue or merge observation. This failure is safer than duplicate submission.
- Concurrent HTTP and event evaluations can race. The durable reservation and
  transaction must remain the correctness boundary.
- New error-kind and action copy requires all five locale catalogs.
- Manual merge behavior must remain unchanged while the automatic path gains
  expected-head enforcement.

## Open questions

None.
