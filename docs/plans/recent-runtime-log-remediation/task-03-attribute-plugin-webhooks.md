---
id: "03-attribute-plugin-webhooks"
title: "Attribute plugin webhook failures"
status: done
wave: 3
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001
  - REQ-PLUGINS-PLUGINS-001
acceptance_criteria:
  - AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.2
  - AC-PLUGINS-PLUGINS-001.4
system_design:
  - ../../specs/platform/system-design/runtime-failure-attribution.md
  - ../../specs/plugins/system-design/plugins-02.md
---

# Task 03: Attribute plugin webhook failures

## Summary

Earlier plugin webhook 503s cannot be assigned to the host or a plugin from
access logs. Classify each 5xx at the dispatch, RPC, or plugin-response
boundary while preserving the exact HTTP response.

## In scope

- Add safe structured origin and status logging in `Controller.webhook`.
- Cover host lease/RPC failure and plugin-supplied 503 with tests.

## Out of scope

- Changing webhook authentication, plugin lifecycle, or external manifests.

## Acceptance

- Logs identify origin and plugin ID for every authenticated/declaration-passed 5xx.
- Webhook key, body, query, headers, and credentials are absent from new logs.
- Response status and body remain unchanged.

## Verification

```bash
(cd apps/backend && go test -tags fts5 ./internal/plugins -run 'TestWebhook' -count=1)
```

PR review follow-up confirmed that `Service.Get` reads the in-memory plugin
registry and returns only the record or `store.ErrNotFound`; the webhook maps
that not-found result to 404 after authorization. The proposed lookup-driven
500 branch is unreachable in the current implementation, so no code change was
needed.

## Files likely touched

- `apps/backend/internal/plugins/handlers.go`
- `apps/backend/internal/plugins/handlers_webhook_lifecycle_test.go`

## Dependencies

None.

## Risks

- Logging before the authorization and declaration gates could reveal an
  installed plugin ID; emit only after those gates.

## Parallelism

`sequential`

## Inputs

- Runtime-failure-attribution design and plugin webhook proxy contract.
- Existing webhook lifecycle and authentication tests.

## Results

Webhook 5xx outcomes now report plugin ID, status, and a fixed origin after the
authorization and declaration gates. Host lifecycle/RPC failures use bounded
error classes; plugin responses carry no host error class. Status and body
semantics are unchanged. Verification passed:

```bash
(cd apps/backend && go test -tags fts5 ./internal/plugins -run 'TestWebhook' -count=1)
```
