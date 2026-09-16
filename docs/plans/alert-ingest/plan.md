---
created: 2026-09-15
status: in_progress
requirements:
  - REQ-INTEGRATIONS-ALERT-INGEST-001
  - REQ-INTEGRATIONS-ALERT-INGEST-002
  - REQ-INTEGRATIONS-ALERT-INGEST-003
system_design:
  - ../../specs/integrations/system-design/alert-ingest.md
legacy_specs: []
---

# Alert Ingest Plan

## Goal

Make an external alert source cost a declaration rather than a package, so
Crashlytics, Datadog, Grafana Alertmanager, Bugsnag, Rollbar, New Relic, Better
Stack and PagerDuty are reachable in days each instead of weeks.

The measured starting point: Sentry is 5,681 production lines, of which
`rest_client.go` plus `models.go` are 995, about 17 percent. The other 83
percent is framework work repeated per vendor. The target for a new source is a
descriptor plus one normalization function, on the order of 150 to 300 lines.

## Definition of done

1. A webhook-capable vendor is usable through configuration alone, with
   deduplication, filtering and repository binding, and no new code.
2. A first-party source is added by registering a descriptor and one
   `Normalize` function, with no new table, handler, settings page or migration.
3. Sentry runs on the shared framework, proving the abstraction fits a source
   Kandev already had rather than only sources chosen to fit it.
4. One new source is delivered on the framework and the work is measured, so the
   days-per-source claim is evidence rather than intent.

## Applicable specifications

- [Alert ingest requirements](../../specs/integrations/requirements/alert-ingest.md)
- [Alert ingest system design](../../specs/integrations/system-design/alert-ingest.md)
- [ADR 2026-09-15 declarative alert ingest](../../decisions/2026-09-15-declarative-alert-ingest.md)

## Delivery tracks

The work runs on two independent tracks. Track A ships user-visible value on its
own and does not wait for Track B.

Seven work orders, not eleven. Options that change one configuration shape ship
as one card, and foundation work that ships nothing alone is not split into two
pull requests of scaffolding. Three cards stay separate on purpose: T00 ships a
security fix today and must not wait for a framework, T08 is the gate, and T09
is the measurement that T08 would destroy if folded into it.

    T00 workspace scoping fix        (independent, pre-existing defect)
         |
         +--> T08

    Track A -- generic webhook path
    T01 webhook alert ingest path    (dedup key + filters + repository
                                      binding + Crashlytics recipe)

    Track B -- alert source framework
    T04 alert model, storage, descriptor, field spec
     |-> T06 registry, ingest, HTTP
           |-> T07 source settings UI
                 |-> T08 port Sentry          (also needs T00)
                       |-> T09 Datadog source

## Dependency order

| Work order | Depends on | Ships value alone |
| --- | --- | --- |
| [T00 issue watch workspace scoping](task-00-issue-watch-workspace-scoping.md) | none | yes |
| [T01 webhook alert ingest path](task-01-webhook-alert-ingest-path.md) | none | yes |
| [T04 alert model and descriptor](task-04-alert-model-and-descriptor.md) | none | no |
| [T06 registry, ingest and HTTP](task-06-registry-ingest-and-http.md) | T04 | no |
| [T07 source settings UI](task-07-source-settings-ui.md) | T06 | no |
| [T08 port Sentry](task-08-port-sentry.md) | T00, T06, T07 | yes |
| [T09 Datadog source](task-09-datadog-source.md) | T08 | yes |

## Risks

- **The abstraction is validated only against new sources.** Mitigated by T08.
  Porting Sentry is the gate, not an optional follow-up. If Sentry does not fit,
  the descriptor model is wrong and T09 must not proceed.
- **The schema-driven settings form is the largest single piece.** It has no Go
  or React precedent in this repository. If T07 overruns, T08 can land with a
  source-specific form and adopt the renderer afterwards, at the cost of
  temporarily keeping one bespoke page.
- **Payload-driven repository binding is a privilege boundary.** The ingest
  route is authorized by a shared secret, not a user identity. T03 must
  constrain resolution to the automation's configured repositories; resolving a
  repository named only in the payload would let a leaked secret choose where an
  agent executes.
- **Migration of Sentry rows is one way.** T08 copies rows and leaves the old
  tables in place. Dropping them is deliberately not in this plan.
- **T01 is four merged concerns in one card.** Dedup, filters, repository
  binding and the recipe ship together because they share one configuration
  shape. If the card overruns, split the recipe out first; it is documentation
  and has no code dependency on the other three.
- **Scope pressure toward incident management.** Correlation, grouping and
  on-call routing are excluded in the requirements and must stay excluded; an
  alert produces at most one task.

## Verification strategy

Every work order states its own commands. Across the initiative:

- Backend: `make -C apps/backend test` and `make -C apps/backend lint`.
- Frontend: `cd apps/web && pnpm run typecheck` and
  `pnpm --filter @kandev/web lint` from `apps/`.
- Specifications: `python3 scripts/list-docs.py validate` and
  `python3 scripts/lint-spec-files.py --all`.
- T08 additionally requires that an existing Sentry watch continues to create
  tasks after migration, verified against a mock Sentry source rather than a
  live instance.
- T09 additionally records the actual line count and elapsed time of adding the
  source, which is the evidence for the plan's goal.

## ASCII UI previews

UI changes appear in T01 and T07. Backend-only work orders carry none.

`UI-01: Alert source settings, rendered from the descriptor schema.`
Entry point: Settings, Integrations, Alert sources. State: one configured
source, one unconfigured type available.

    +--------------------------------------------------------------+
    | Alert sources                              [ + Add source ]   |
    +--------------------------------------------------------------+
    | [icon] Datadog - Production            Connected    [ Edit ]  |
    |        3 watches - last alert 4m ago                          |
    +--------------------------------------------------------------+
    | [icon] Sentry - kegmil                 Auth required [ Fix ]  |
    |        1 watch - last error 2h ago                            |
    +--------------------------------------------------------------+

    Add source dialog, fields generated from the field spec:

    +--------------------------------------------------------------+
    | Add alert source                                              |
    |                                                               |
    | Type      [ Datadog             v ]                           |
    | Name      [ Production            ]                           |
    | API token [ ....................  ]  (declared Secret)        |
    | Site      [ datadoghq.com         ]  (declared Default)       |
    | > Advanced                                                    |
    |                                                               |
    |                             [ Cancel ]  [ Add source ]        |
    +--------------------------------------------------------------+

Structural requirements: field order follows the descriptor; a field declared
`Secret` renders masked and is never populated from a read; fields declared
`Advanced` are collapsed by default. Spacing here is illustrative, not a pixel
specification. Copy is localized in five languages per the repository rule.
