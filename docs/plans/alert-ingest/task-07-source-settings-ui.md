---
id: "07-source-settings-ui"
title: "Schema-driven alert source settings UI"
status: pending
wave: 2
depends_on:
  - "06-registry-ingest-and-http"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-ALERT-INGEST-002
acceptance_criteria:
  - AC-INTEGRATIONS-ALERT-INGEST-002.3
  - AC-INTEGRATIONS-ALERT-INGEST-002.4
system_design:
  - ../../specs/integrations/system-design/alert-ingest.md
---

# T07: Schema-driven alert source settings UI

## Outcome

One settings surface configures every alert source, rendering each form from the
descriptor schema, so a new source type needs no frontend work.

## In scope

- Alert sources list and add/edit dialog under Settings, Integrations.
- A generic form renderer driven by the schema served from
  `GET /api/v1/alert-sources/types`.
- Field rendering honoring `Secret` (masked, never populated from a read),
  `Default`, `Description`, `Optional` and `Advanced` (collapsed).
- Reuse of the existing integration shells: `IntegrationAuthStatusBanner` and
  `IntegrationAuthErrorMessage`.
- Hooks under `hooks/domains/alertsource/`, following the repository rule that
  hooks do not live under `components/`.
- All copy localized in the five supported languages.

## Exclusions

- No per-source custom components. A source that cannot be expressed in the
  field spec is a field spec gap and is fixed in T05.
- No watch editing UI beyond what the existing watcher surfaces already provide.

## Applicable specifications

- `REQ-INTEGRATIONS-ALERT-INGEST-002`, `AC-INTEGRATIONS-ALERT-INGEST-002.3`,
  `AC-INTEGRATIONS-ALERT-INGEST-002.4`
- [Alert ingest system design](../../specs/integrations/system-design/alert-ingest.md),
  Components.

## Implementation acceptance conditions

1. Adding a new source type to the backend registry causes it to appear in the
   type selector with a working form, with no frontend change.
2. A field declared `Secret` renders masked, submits a new value when changed,
   and is never populated from a read response.
3. A field declared `Advanced` is collapsed by default and does not block
   submission when left at its default.

## Verification

    cd apps/web && pnpm run typecheck
    cd apps && pnpm --filter @kandev/web lint
    cd apps/web && pnpm run i18n:check
    cd apps/web && pnpm run i18n:ratchet

## Likely files

- `apps/web/components/integrations/alert-sources/`
- `apps/web/hooks/domains/alertsource/`
- `apps/web/src/locales/*/` for the five catalogs

## ASCII UI previews

See `UI-01` in [plan.md](plan.md). The list and dialog specified there are this
work order's deliverable. Structural requirements restated: field order follows
the descriptor; `Secret` renders masked; `Advanced` collapses.

## Dependencies

T05 for the schema shape, T06 for the endpoints that serve it.

## Results

Not started.
