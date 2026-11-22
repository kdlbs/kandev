---
id: "01-access-and-trust"
title: "Focused access and host review"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-CONNECTIONS-002
  - REQ-EXECUTORS-SSH-CONNECTIONS-004
acceptance_criteria:
  - AC-EXECUTORS-SSH-CONNECTIONS-002.2
  - AC-EXECUTORS-SSH-CONNECTIONS-002.3
  - AC-EXECUTORS-SSH-CONNECTIONS-004.3
  - AC-EXECUTORS-SSH-CONNECTIONS-004.12
  - AC-EXECUTORS-SSH-CONNECTIONS-004.15
  - AC-EXECUTORS-SSH-CONNECTIONS-004.16
  - AC-EXECUTORS-SSH-CONNECTIONS-004.17
  - AC-EXECUTORS-SSH-CONNECTIONS-004.19
system_design:
  - ../../specs/executors/system-design/ssh-connection-management-ui.md
  - ../../specs/executors/system-design/ssh-connection-manager.md
---

# Task 01: Focused access and host review

## Summary

Replace competing access controls and the small trust popover with one focused
task and complete inline host review. Establish operation-local feedback while
preserving admission, route evidence, global load errors and existing enrollment.

## In scope

Access milestones and next-action projection; incomplete/ready/failure states;
inspection pending/error/expiry; old/new fingerprint review; exact public copy;
focus and responsive invalidation; narrowly scoped operation/error metadata.
Translate new copy in all locales. Keep the existing installation callable until
Task 02 replaces its internal composition.

## Out of scope

Enrollment redesign, diagnostics report, backend/API changes, shared global
confirmation redesign, automatic probes, live trust or instance changes.

## Acceptance

1. Access shows one current task and primary action. Working connections do not
   require enrollment. Inspection, test and trust run only from explicit actions.
2. Inline review labels full target/fingerprint evidence and a changed-key
   warning. Expired/stale/changed targets cannot be confirmed. Back, Escape and
   responsive decision changes invalidate review and restore focus.
3. Local operation status/error does not duplicate global feedback or hide
   unrelated errors. Desktop/phone geometry, localization and one-submit guards
   pass. Existing enrollment and discovery behavior stays usable.

## UI preview

[UI-01 full preview](plan.md#ui-01-access-and-host-trust), AC-004.15/16/17/19.
Structured preview follows the task's no-ASCII instruction.

| State | Content | Main action |
| --- | --- | --- |
| Needs trust | One task, target, comparison explanation | Inspect host key |
| Observed | Labelled account and server fingerprint; old/new warning when changed | Trust this server |
| Working | Local indicator with stable command name | Duplicate action disabled |
| Ready | One green result; secondary Host details/Install | Test SSH access |

Phone shows the current milestone and completed summary, not compressed desktop
steps. Review stays on the page. One shell scroller and full-width primary action.

## Verification

Use TDD for proposed `access-experience.test.tsx`, feedback routing and new state
projection. Preserve `connection-presentation.test.ts`, `access-flow.test.tsx`
and hook admission tests. Add named cases: no command on entry; changed/expired
observation; target/viewport invalidation; no duplicate alert; load error fallback;
late response from another connection; current ready test and optional install.
Adapt existing desktop/mobile access helpers without weakening their assertions.

Commands run from repo root, projects sequentially. Install locked dependencies
if missing. Build fresh web/backend before browser checks. UI fixtures must not
call the user's manual lab. Before completion inspect light/dark, pending/error,
pseudo, full fingerprint copy, 767/768px and coarse-tablet screenshots.

```bash
(cd apps/web && pnpm exec vitest run components/settings/ssh-connections hooks/domains/settings/use-ssh-connections.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings components/settings/ssh-connections hooks/domains/settings/use-ssh-connections.ts hooks/domains/settings/use-ssh-authentication.ts e2e/helpers/ssh-access-setup.ts e2e/tests/settings/ssh-access-setup.spec.ts e2e/tests/settings/mobile-ssh-access-setup.spec.ts)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
make build-backend build-web
pnpm --dir apps/web e2e:run --host --project chromium tests/settings/ssh-access-setup.spec.ts tests/settings/ssh-management-layout.spec.ts
pnpm --dir apps/web e2e:run --host --project mobile-chrome tests/settings/mobile-ssh-access-setup.spec.ts tests/settings/mobile-ssh-management-layout.spec.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

Paths relative to `apps/web/`:
`components/settings/ssh-connections/{connection-access,connection-trust,settings,manager-detail}.tsx`;
proposed `access-progress.tsx`, `host-trust-review.tsx`, `access-experience.test.tsx`;
existing presentation/access tests; `hooks/domains/settings/use-ssh-connections.ts`
and authentication hook only where required for safe error context;
`e2e/helpers/ssh-access-setup.ts`, desktop/phone access specs, affected settings
locales. Search all SSH tests for old Trust confirmation selectors and include
any additional changed suites in the verification record.

## Dependencies

None. Existing uncommitted discovery/status implementation must be preserved.

## Risks

Detached error messages hidden accidentally; pending same-tick admission;
new inline review accidentally weakening target or expiry checks; replacing
current verified evidence with an optimistic green state.

## Parallelism

`sequential`

## Inputs

Owning requirement ACs above; interaction design's proposed Access and Operation
feedback sections; source and baseline captures listed in plan. Apply TDD,
mobile-parity, better-ui, e2e and clear-English communication skills.

## Results

Implemented inline trust review, labelled full fingerprints, the changed-key warning, Access milestones, bastion setup navigation, and route-scoped operation feedback. Added RED/GREEN coverage for stale decisions, responsive invalidation, duplicate admission, and keyboard focus after the server changes a route stamp. Final unit and desktop/phone browser checks passed. Trust screenshots were inspected in both themes. See [verification](verification.md).
