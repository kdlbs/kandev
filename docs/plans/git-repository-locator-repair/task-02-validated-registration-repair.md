---
id: "02-validated-registration-repair"
title: "Validate and persist registration clone-URL changes"
status: done
wave: 2
depends_on:
  - "01-scp-workspace-locators"
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-002
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
acceptance_criteria:
  - AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-002.1
  - AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-002.2
  - AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-002.3
  - AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-002.4
  - AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-002.6
  - AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-002.7
  - AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-002.8
  - AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-002.9
  - AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-002.10
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.20
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.21
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.22
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.23
system_design:
  - ../../specs/workspaces/system-design/remote-repository-registration.md
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
---

# Task 02: Validate and Persist Registration Clone-URL Changes

## Summary

Implement the domain and persistence boundary for #5188.
Validate a supplied URL against the authorized saved identity, expose a write-free preview, and protect the committed repair from stale ordinary saves.
Public HTTP/WS exposure and MCP preview wiring belong to Task 03. Existing MCP writes immediately gain service validation here.

## In scope

- Add `Service.ValidateRepositoryRemoteURLUpdate` and its bounded, redacted preview result.
- Authorize before provider calls. Enforce the owning workspace's read-only policy for this operation.
- Reuse pinned preflight and active plugin inspection, then compare every saved identity component.
- Validate strict built-in SCP identity through its HTTPS equivalent without widening discovery. Retain requested SCP bytes and refuse unprovable scheme/port mappings.
- Reject unknown host/identity, provider/namespace/scope mismatch, credentials, local paths, and permissive browser-URL truncation.
- Support complete GitHub/GitLab identities and exact plugin descriptors. Apply the design's conditional Azure rule without guessing missing organization evidence.
- Apply the validator to every supplied `UpdateRepositoryRequest.RemoteURL`, including exact service saves.
- Extend ordinary write intent and its binding companion with optional remote URL and an internal observed-identity fence.
- Preserve omitted `remote_url` at SQL assignment time. Capture committed URL, checkout pair, and timestamp before response/event publication.
- Add safe clone-URL projection to `publishRepositoryEvent`. Keep errors and unsafe legacy locators out of new output fields.
- Preserve active checkout omission, binding atomicity, exact administration, direct complete writes, and managed SSH identity normalization.

## Out of scope

- HTTP/WS request changes, new MCP tools, general all-field patch semantics, migrations, and provider identity repair.
- Git calls, origin edits, checkout reconciliation, task/session creation or restart, and credential/journal changes.
- Changing registration's find-or-create/backfill behavior or historical companion plan results.

## Acceptance

1. Preview is write-free. Same-identity updates preserve registration/source/task identity, and malformed, unauthorized, mismatched, or changed plugin descriptors cause no writes/events.
2. Causal independent-connection tests prove omitted URL preservation, committed response/event values, identity-race conflicts, and atomic settings/binding rollback on SQLite and PostgreSQL.
3. Existing checkout-default, exact-save, registration/backfill, and managed SSH credential controls retain their behavior without a schema migration.

## TDD and verification

Start with RED tests for an invalid URL reaching `UpdateRepository` and a stale ordinary settings save reversing a URL repair.
Use real temporary SQLite storage and actual service event publication.
Then cover the preview, plugin identity, denied/read-only access, and no-Git side-effect boundaries.
Gate the inspection-to-write race causally. Do not rely on sleeps.

Run from the repository root:

```bash
(cd apps/backend && go test -trimpath -race ./internal/task/service ./internal/task/repository/sqlite -run 'Test(RepositoryRemoteURL|RepositoryCheckoutDefaults|Service_FindOrCreateRepository.*SSH)' -count=1)
(cd apps/backend && go test -trimpath ./internal/gitcredentials ./internal/repoclone -run 'Test(ResolveRepositoryIdentity|CanonicalHTTPSCloneURL|ValidateHTTPSCloneOrigin)' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The PostgreSQL step requires a disposable database selected by the existing test DSN:

```bash
set -e
test -n "${KANDEV_TEST_POSTGRES_DSN:-}"
(cd apps/backend && go test -trimpath -race ./internal/task/repository/sqlite -run '^TestRepositoryRemoteURLPhysicalPostgres$' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)
```

If the DSN is absent, record a blocked PostgreSQL gate rather than counting skipped tests as passed.
Use the existing isolated-schema and multi-connection harness. Do not point it at a user database.
Cover both stale-write orders, two explicit valid URLs, identity mutation during inspection, deletion, cancellation, binding failure, and exact timestamp conflicts.

Run scoped lint after the targeted tests, with the actual changed Go file list:

```bash
git diff --name-only --diff-filter=ACMR -- '*.go' > /tmp/kandev-locator-changed-go.txt
git ls-files --others --exclude-standard -- '*.go' >> /tmp/kandev-locator-changed-go.txt
xargs -r bash scripts/lint-go-changed < /tmp/kandev-locator-changed-go.txt
```

## Files likely touched

- `apps/backend/internal/task/service/service_resources.go`, `service_requests.go`, and new `service_repository_remote_url.go`
- New `apps/backend/internal/task/service/service_repository_remote_url_test.go` and concurrency/access companions
- `apps/backend/internal/task/service/service_events.go`
- `apps/backend/internal/task/models/repository_checkout_intent.go` and repository interfaces/aliases that carry its intent
- `apps/backend/internal/task/repository/sqlite/repository_checkout_defaults.go`
- New SQLite and physical PostgreSQL `repository_remote_url*_test.go` files
- Exact service/store helper paths only as needed to preserve supplied-URL validation and existing timestamp fences

## Dependencies

Task 01 supplies the strict conventional SCP parser.
Existing configured-origin verification, `RepositorySelectionResolver`, and checkout-intent storage are present on the analyzed main.
Re-read their current contracts before implementation if main moves.

## Risks

- Plugin resolution ignores mutable input hints. Comparison to the saved descriptor after inspection is essential.
- GitHub/GitLab URL parsers accept browser resource shapes. Repair must reject truncation and ambiguous separators before those parsers.
- Exact reads and direct complete writes must not silently become ordinary partial patches.
- Unknown legacy hosts and incomplete Azure organization identity are intentionally unrepairable here.
- PostgreSQL tests must use physical independent connections, not SQLite or mocked statements.

## Parallelism

`sequential`

## Inputs

- [Repair criteria](../../specs/workspaces/requirements/remote-repository-registration.md#clone-url-repair)
- [Repair design](../../specs/workspaces/system-design/remote-repository-registration.md#clone-url-repair)
- [Active checkout-default design](../../specs/workspaces/system-design/worktree-base-refresh.md#repository-checkout-settings-persistence)
- [Validated-update ADR](../../decisions/2026-11-08-validated-repository-locator-updates.md)
- Existing checkout-default service/store/PostgreSQL tests, plugin selection adapter tests, and backend test guidance

## Results

Implemented on 2026-11-08.

- `Service.ValidateRepositoryRemoteURLUpdate` performs authorization, read-only refusal, strict locator admission,
  saved-identity pinning, built-in origin verification, plugin descriptor comparison, and bounded preview.
- `UpdateRepository` validates every supplied `RemoteURL` again and carries an internal identity fence through the
  ordinary write intent. Omitted URLs are preserved at the SQL assignment boundary, and the committed URL is projected
  into the response and `repository.updated`.
- `TestRepositoryRemoteURLValidation`, `TestRepositoryRemoteURLPreviewIsWriteFree`, access, plugin, GitLab, and Azure
  tests pass. `TestRepositoryRemoteURLConcurrentSQLite` covers both ordinary-save orders, two valid repairs, an identity
  race, and binding rollback. Store-level omission, fence, atomicity, and legacy/exact tests pass.
- `TestRepositoryRemoteURLPhysicalPostgres` is present and skips cleanly because `KANDEV_TEST_POSTGRES_DSN` is absent.
  The physical PostgreSQL gate remains unexecuted.
- Review follow-up: repair now requires a nonempty persisted provider origin before normalization, rejects conflicting legacy Azure hosts, preserves exact saved identity bytes when an equivalent padded field is supplied, and accepts supported encoded or Unicode HTTPS paths while rejecting encoded separators, controls, and traversal.
  Repair also compares the proposed URL's exact scheme and authority with the saved origin before retaining raw encoded bytes, and safe encoded paths remain displayable in previews and repository events.
  Deliberately complete provider-identity updates and local-source metadata round trips remain on the existing full-update path; saved-identity pinning applies to ordinary clone-URL repair.
