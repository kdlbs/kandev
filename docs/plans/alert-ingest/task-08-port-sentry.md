# T08: Port Sentry onto the alert source framework

## Outcome

Sentry runs as a descriptor plus a normalization function on the shared
framework, proving the abstraction fits a source Kandev already had.

This is the gate for the initiative. If Sentry does not fit, the descriptor
model is wrong and T09 must not proceed.

## In scope

- A Sentry descriptor declaring its configuration fields, fingerprint fields and
  watch metadata key, preserving the existing `sentry_issue_watch_id` value so
  in-flight counting of existing tasks is unbroken.
- `Normalize` mapping a Sentry issue to the `Alert` model, reusing the existing
  REST client unchanged.
- `Enrich` fetching the issue's latest event for stack context.
- Multi-instance support: Sentry allows several named instances per workspace,
  and `alert_sources` already models this as several rows, so the bound-instance
  behavior maps to a source id.
- One-way migration copying `sentry_configs`, `sentry_issue_watches` and
  `sentry_issue_watch_tasks` into the shared tables, with secrets rekeyed to the
  new source ids.
- Removal of the Sentry-specific store, handlers, poller, mock controller and
  settings page once the ported path passes.

## Exclusions

- The Sentry tables are not dropped. Migration copies; the old tables remain for
  one release.
- No change to the Sentry REST client or its issue models.
- No port of Jira, Linear, GitHub, GitLab or Azure DevOps.

## Applicable specifications

- `REQ-INTEGRATIONS-ALERT-INGEST-002`, `REQ-INTEGRATIONS-ALERT-INGEST-003`
- `AC-INTEGRATIONS-ALERT-INGEST-003.4`, `AC-INTEGRATIONS-ALERT-INGEST-003.5`,
  `AC-INTEGRATIONS-ALERT-INGEST-003.6`
- [Alert ingest system design](../../specs/integrations/system-design/alert-ingest.md),
  Persistence.

## Implementation acceptance conditions

1. An existing Sentry watch continues to create tasks after migration, verified
   against the mock source, with its `max_inflight_tasks` still enforced.
2. The per-watch limit is enforced for Sentry through the shared path, closing
   the defect where the limit was stored and validated but never applied.
3. Listing Sentry sources and watches returns only the caller's workspaces,
   carried forward from T00 rather than reintroduced.

## Verification

    make -C apps/backend test
    make -C apps/backend lint
    cd apps/backend && go test ./internal/integrations/alertsource/... ./internal/sentry/... -count=1
    cd apps/web && pnpm run typecheck

Record the production line count of the Sentry source after the port, for
comparison with the 5,681 line starting point.

## Likely files

- `apps/backend/internal/integrations/alertsource/sources/sentry/`
- `apps/backend/internal/sentry/` (store, handlers, poller, mock controller
  removed; `rest_client.go` and `models.go` retained)
- Migration alongside the shared store

## Dependencies

T00, T06, T07.

## Results

Not started.
