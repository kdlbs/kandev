---
created: 2026-10-18
status: implemented
requirements:
  - REQ-INTEGRATIONS-LINEAR-PROJECTS-001
  - REQ-INTEGRATIONS-LINEAR-PROJECTS-002
system_design:
  - ../../specs/integrations/system-design/linear-watch-projects.md
legacy_specs: []
---

# Implementation Plan: Linear Watch Project Filter

## Overview

Add a project filter to Linear issue watches and project placeholders to
watch prompts ([issue #4557](https://github.com/kdlbs/kandev/issues/4557)).
Two work orders deliver independent results: routing by project, then project
context in the prompt. Both follow the
[owning design](../../specs/integrations/system-design/linear-watch-projects.md).

## Evidence

On `main` at `a8e982cff`, `issueFragment` requests no project,
`SearchFilter` has no project field, `interpolateLinearPrompt` has no project
placeholders, and the watch dialog builds the whole filter from its form
state, so a project filter set through the API would be dropped by the next
save from the UI. Linear's public GraphQL schema names the short summary
`Project.description` and the markdown body `Project.content`; there is no
`summary` field.

## Scope

### In scope

- `REQ-INTEGRATIONS-LINEAR-PROJECTS-001` and `-002` with all their criteria.
- Linear client, mock client, watch service, HTTP route, orchestrator prompt
  interpolation, watch dialog, placeholders, locales, and public docs.

### Out of scope

- Project filters in the `/linear` browser or quick launch.
- Changes to the default prompt, storage schema, or other providers.

## Work orders

- [x] [Task 01: Filter Linear watches by project](task-01-project-filter.md)
- [x] [Task 02: Project context in Linear watch prompts](task-02-project-prompt-context.md)

Task 02 depends on Task 01 for the issue project fields.

## ASCII UI preview

`UI-01: Linear watch dialog, filter section` — entry point: Settings →
Integrations → Linear → New watcher (or a watcher row). State: team selected,
one project toggled. Structural requirement: a Projects row directly under
Labels, with the same chip control; details are illustrative.

```text
Labels
Click to toggle. Matches ANY of the selected labels.
[ bug ] [ frontend ] [ backend ]

Projects
Click to toggle. Matches ANY of the selected projects.
[*Checkout*] [ Search ] [ Mobile app ]

Estimate min        Estimate max
[        ]          [        ]
```

Without a team the row shows "Pick a team to choose specific projects."
Saved selected IDs remain visible as removable unavailable-project chips;
with no selection, no chips or inventory request appear.
While loading it shows "Loading projects…"; with no projects, "No projects
available for this team." The phone layout is the same single column; chips
wrap. Maps to `AC-INTEGRATIONS-LINEAR-PROJECTS-001.3`, verified by the desktop
and mobile Playwright specs.

## Verification

```bash
(cd apps/backend && go test -race -tags=fts5 -count=1 ./internal/linear/ ./internal/orchestrator/)
bash scripts/lint-go-changed <changed Go files>
(cd apps/web && pnpm exec vitest run components/linear/ && pnpm run typecheck && pnpm run i18n:check)
(cd apps/web && e2e/scripts/run-e2e.sh --host --no-build --shards 1 --project chromium -- tests/integrations/linear-watcher-project-filter.spec.ts)
(cd apps/web && e2e/scripts/run-e2e.sh --host --no-build --shards 1 --project mobile-chrome -- tests/integrations/mobile-linear-watcher-project-filter.spec.ts)
python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all
```

## Verification results

Original author-reported results are recorded in each work order. Review
remediation adds real `FilterFields` regressions and desktop/phone removal
flows for project-only saved watches. Hosted caller assertion RED, shared
controls, and corrected GREEN are recorded below. Full hosted CI and browser
results must be verified against the final source before delivery.

## Risks

- Prompts that literally use `{{issue.project.description}}` add batched Linear
  requests for distinct projects in unseen issues (at most 25 IDs per request).
  This does not bound response bytes; an oversized body fails the whole poll. Watches without the placeholder are
  unchanged.
- Project descriptions are captured at poll time and are not refreshed for
  running tasks.


Hosted review proof: five shared controls passed with the original caller,
then the project-only selection assertion failed because the selected chip was
absent. Restoring the corrected caller passed all 26 selected form/component
tests. The combined proof workflow subsequently failed its standalone typecheck
with the default Node heap exhausted and reported required test formatting.
The hosted formatter output was applied with only wrapping and trailing commas.
This is behavioral RED/controls/GREEN evidence, not a successful typecheck or
full-CI claim. Actual final full frontend typecheck and desktop/phone browser
results remain pending in retained PR evidence.

For head `0e89cdce29`, the full frontend workflow `39155688189` and owning Linear
backend package in `39155688188` passed at hosted merge source `5d27bfe0da`.
The phone run exposed a fixture setup error: omitting `teamKey` allowed the API
helper to supply `ENG`, so the project-only case never opened its intended
dialog. The corrected fixture explicitly supplies an empty team and verifies
the persisted team and project IDs before UI interaction. This timeout is not
product assertion RED; final browser and full-family verification remains
pending for the corrected source.
