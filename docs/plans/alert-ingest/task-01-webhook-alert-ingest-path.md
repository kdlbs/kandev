---
id: "01-webhook-alert-ingest-path"
title: "Webhook alert ingest path"
status: done
wave: 0
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-ALERT-INGEST-001
acceptance_criteria:
  - AC-INTEGRATIONS-ALERT-INGEST-001.1
  - AC-INTEGRATIONS-ALERT-INGEST-001.2
  - AC-INTEGRATIONS-ALERT-INGEST-001.3
  - AC-INTEGRATIONS-ALERT-INGEST-001.4
  - AC-INTEGRATIONS-ALERT-INGEST-001.5
  - AC-INTEGRATIONS-ALERT-INGEST-001.6
  - AC-INTEGRATIONS-ALERT-INGEST-001.7
  - AC-INTEGRATIONS-ALERT-INGEST-001.8
system_design:
  - ../../specs/integrations/system-design/alert-ingest.md
---

# T01: Webhook alert ingest path

## Outcome

The automation webhook trigger becomes a usable alert ingest path: a vendor that
can POST JSON produces deduplicated, filtered, repository-bound tasks through
configuration alone, with no code per vendor. Ships with a worked Firebase
Crashlytics recipe that proves the three options compose.

This is one card rather than four because all three options change the same
trigger configuration shape and the same three files, and because a recipe that
exercises them is the honest acceptance evidence for the feature.

## In scope

**Dedup key.** A `dedup_key` dot path on the webhook trigger config, resolved
against the payload with the existing `lookupPath` helper and passed into
`FireTrigger`. Record whether the key resolved, so an unresolved path is visible
rather than looking like an unconfigured one.

**Filters.** A `filters` list of `{path, op, values}` predicates evaluated
before admission, so a filtered payload consumes no concurrency slot and records
no dedup key. Operators: `eq`, `ne`, `in`, `not_in`, `exists`, `not_exists`,
`contains`. All must pass to fire; a rejection records the predicate that caused
it.

**Repository binding.** A `repository.selector_path` whose resolved value
selects among the automation's already-configured repositories. Generalize
`resolveAutomationRepository` so its payload-driven branch is not gated on
`github_pr`, while constraining the webhook branch to the configured set.

**Crashlytics recipe.** A page under `docs/public/` covering the Cloud Function
on `onNewFatalIssuePublished`, the trigger configuration using all three options
above, and a default prompt that fetches stack context through the Firebase MCP
server and treats alert text as data.

## Exclusions

- No expression language. A predicate list is sufficient for the named sources;
  a CEL-style evaluator is explicitly out of scope.
- No filtering on request headers. The JSON body only.
- No change to `admitTriggerLocked`, which already enforces a non-empty key
  correctly.
- No Crashlytics descriptor, poller or credential storage. Crashlytics exposes
  no issue list method and its read API is `v1alpha`, so there is nothing to
  poll; it is a webhook source by design.

## The security constraint

The webhook endpoint is exempt from session authentication and authorized by a
shared secret alone. `resolveAutomationRepository`'s existing payload-driven
branch is safe for `github_pr` only because Kandev's own poller produced that
payload. A webhook payload is produced by whoever holds the secret.

So the payload **selects among** configured repositories and never resolves a
new one. The webhook path must not call `repositoryResolver.ResolveForReview`
with an owner and name taken from the payload.

## Applicable specifications

- `REQ-INTEGRATIONS-ALERT-INGEST-001`, all of `AC-INTEGRATIONS-ALERT-INGEST-001.1`
  through `AC-INTEGRATIONS-ALERT-INGEST-001.8`
- [Alert ingest system design](../../specs/integrations/system-design/alert-ingest.md),
  Track A and Security sections.

## Implementation acceptance conditions

1. Two POSTs resolving the dedup path to the same value create one task; the
   second is recorded as skipped with its reason. A trigger with no `dedup_key`
   behaves exactly as today.
2. A payload failing any predicate creates no task, records the rejecting
   predicate, and consumes no concurrency slot.
3. A payload naming an `owner/name` outside the automation's configured
   repositories results in no repository binding. A test asserts this with the
   security reason in its name.

## Verification

    make -C apps/backend test
    make -C apps/backend lint
    cd apps/backend && go test ./internal/automation/... ./internal/orchestrator/... -count=1
    python3 scripts/list-docs.py validate

Manual, for the recipe: configure the automation against a staging Firebase
project; confirm one task per new fatal issue and no second task on redelivery.

## Two prerequisites the recipe must state, not assume

1. Kandev must be reachable from Google's network. A Cloud Function cannot reach
   `localhost:8817`. Document the options and note that exposing the whole port
   is unsafe when Kandev runs with auth disabled, because the webhook secret
   protects only that one route.
2. React Native apps must upload JS source maps. Without them the agent receives
   minified frames such as `index.android.bundle:1:13` and can name the error
   class but not the file.

## Likely files

- `apps/backend/internal/automation/webhook.go` (currently passes `""`)
- `apps/backend/internal/automation/models.go`
- `apps/backend/internal/automation/trigger_registry.go`
- `apps/backend/internal/automation/interpolator.go` (`lookupPath`, reused)
- New predicate evaluator alongside `interpolator.go`
- `apps/backend/internal/orchestrator/event_handlers_automation.go`
- `docs/public/` recipe page

## ASCII UI previews

The automation edit form gains three optional fields in the existing webhook
trigger section:

    Webhook trigger
      Dedup key path     [ data.issueId                ]
      Repository from    [ data.service                ]   (optional)
      > Filters          [ data.severity  in  fatal,error ]

Structural requirement: the repository field offers only repositories already
selected on the automation; it is not a free-text repository reference. Copy is
localized in five languages.

## Dependencies

None. This is the first shippable increment of the initiative.

## Results

Not started.
