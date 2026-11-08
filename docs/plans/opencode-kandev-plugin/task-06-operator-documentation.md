---
id: "06-operator-documentation"
title: "Document activation, limitations, and compatibility evidence"
status: completed
wave: 6
depends_on:
  - "05-browser-executor-proof"
plan: "plan.md"
requirements:
  - REQ-AGENTS-OPENCODE-PLUGIN-001
  - REQ-AGENTS-OPENCODE-PLUGIN-002
  - REQ-AGENTS-OPENCODE-PLUGIN-004
  - REQ-AGENTS-BACKGROUND-WORK-007
acceptance_criteria:
  - AC-AGENTS-OPENCODE-PLUGIN-001.2
  - AC-AGENTS-OPENCODE-PLUGIN-002.1
  - AC-AGENTS-OPENCODE-PLUGIN-004.1
  - AC-AGENTS-OPENCODE-PLUGIN-004.3
  - AC-AGENTS-OPENCODE-PLUGIN-004.4
  - AC-AGENTS-OPENCODE-PLUGIN-004.6
  - AC-AGENTS-BACKGROUND-WORK-007.8
  - AC-AGENTS-BACKGROUND-WORK-007.9
system_design:
  - ../../specs/agents/system-design/opencode-kandev-plugin.md
---

# Task 06: Document activation, limitations, and compatibility evidence

## Summary

Publish only behavior proved by the earlier work orders. Document automatic
plugin supply, operator activation, supported v2 versions, and the status-only
scope. Reconcile implementation records without claiming upstream continuity.

## Scope and owned files

- Own the relevant sections of `docs/public/agents-and-profiles.md`,
  `docs/public/tasks-and-workflows.md`, and `docs/public/configuration.md`.
  Update the existing background-work explanation in place.
- Own a short developer README in `apps/packages/opencode-plugin` that explains
  the entry point, module contract, artifact build, conformance, and cleanup.
- Own completion results in this package's plan, work orders, and evidence file.
- Reconcile the paired requirements/design and the bounded companion references
  in `docs/plans/opencode-background-continuity`.

Treat configuration documentation as reference. Treat provider behavior and
limitations as explanation. Do not publish speculative implementation detail as
an operator procedure. Public documentation changes only after the described
behavior exists.

Document `features.opencodePlugin` / `KANDEV_FEATURES_OPENCODE_PLUGIN`, the initial
off defaults, restart requirement, and effective override precedence. When the
operator enables it, Kandev supplies and starts the plugin on eligible launches.
Users do not install an npm package. V1 and unsupported runtimes retain ACP.

State that terminal jobs can settle after the foreground prompt. Intermediate
unknown activity remains possible. Output streaming, controls, automatic
assistant follow-ups, and historical-job recovery remain unavailable here.

Describe how a future behavior adds a typed module and conformance evidence.
Keep the shared core free of behavior-specific transport branches. Explain that
this OpenCode asset is separate from Kandev marketplace plugins.

After all work orders pass, promote only the new plugin requirement/design pair
and mark this plan implemented. Do not promote the entire shared background-work
requirement or full-continuity design while their upstream scope remains pending.

## Acceptance

1. Operator documentation matches actual activation, compatibility, and fallback
   evidence. It contains no manual plugin-install step or full-continuity claim.
2. Developer documentation explains adding a behavior through the existing
   lifecycle and typed protocol. It requires no arbitrary command mechanism.
3. Results and statuses agree across both packages. Initial defaults remain off,
   and the upstream producer tasks remain blocked unless independent proof exists.

## Verification

From the repository root:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Search public docs, the root README, and `docs/screenshots.md` for existing
OpenCode/background-work claims. Update affected claims in place. Do not rewrite
generated release history or add screenshots without a demonstrated need.

## Exclusions and dependencies

Task 05 must provide actual results. This work order is documentation-only and
needs no artificial TDD test. It does not repeat all implementation suites or
add generic QA/review gates. Promotion, flag retirement, publication, and a PR
remain separate user-controlled actions.

## Results

Completed on 2026-10-29.

- Updated the operator explanation in `docs/public/agents-and-profiles.md` and
  `docs/public/tasks-and-workflows.md`, including automatic supply, opt-in
  activation, supported version, fallback, and the status-only scope.
- Added the feature toggle row and precedence/restart note in
  `docs/public/configuration.md`.
- Added `apps/packages/opencode-plugin/README.md` with the entry point, behavior
  contract, artifact build, and packaged conformance commands.
- Reconciled the requirements, system design, plan, work orders, and evidence
  file. Public scope does not claim automatic assistant responses, liveness,
  output streaming, controls, or historical recovery.
