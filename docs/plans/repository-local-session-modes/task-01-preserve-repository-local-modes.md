---
id: "01-preserve-repository-local-modes"
title: "Preserve repository-local modes"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-REPOSITORY-LOCAL-MODE-001
acceptance_criteria:
  - AC-AGENTS-REPOSITORY-LOCAL-MODE-001.1
  - AC-AGENTS-REPOSITORY-LOCAL-MODE-001.2
  - AC-AGENTS-REPOSITORY-LOCAL-MODE-001.3
  - AC-AGENTS-REPOSITORY-LOCAL-MODE-001.4
  - AC-AGENTS-REPOSITORY-LOCAL-MODE-001.5
  - AC-AGENTS-REPOSITORY-LOCAL-MODE-001.6
system_design:
  - ../../specs/agents/system-design/repository-local-session-modes.md
---
# Task 01: Preserve repository-local modes

## Summary and scope

Deliver the profile picker and scratch-probe behavior in the linked design.
Keep the existing runtime confirmation and profile persistence boundaries.
Model fallback and provider-specific public APIs are outside this work order.

## Acceptance

- Custom IDs remain editable through save and reload on desktop and phone.
- The host probe respects legacy and ConfigOptions advertisements, including
  empty and grouped lists, while real sessions confirm explicit modes.
- Search clears on every close path, and discovery readiness still gates edits.

## ASCII UI preview

UI-01 is the profile mode row in [the plan](plan.md#ascii-ui-preview).

```text
Desktop: Start model [model]   Start mode [reviewer v]   [refresh]
Phone:   Start model [model] [refresh]
         Start mode  [reviewer v]
Open:    [Search modes...]  reviewer Custom
```

AC-001.1, .5, and .6 govern the interaction. The phone row uses the existing
settings scroll owner and touch sizing; spacing is illustrative.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -race ./internal/agentctl/server/utility -run '^TestProbeACPSessionWithContext' -count=1)
(cd apps/web && pnpm exec vitest run components/settings/mode-combobox.test.tsx)
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --project chromium e2e/tests/settings/agent-profile-custom-mode.spec.ts
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --project mobile-chrome e2e/tests/settings/mobile-profile-custom-mode.spec.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files and dependencies

The work touches the agentctl utility probe and its tests, profile mode picker
and capability row, localized agent catalogs, and the profile E2E files. It has
no work-order dependencies and proceeds sequentially. The public profile guide
documents mode entry and task startup confirmation.

## Risks

Host discovery cannot validate a repository-local mode. The task executor must
continue to confirm the requested value before sending the first prompt.

## Results

The contribution and maintainer follow-up are implemented. The verification
commands above passed: the focused branch Go probe tests under the race detector,
six Vitest tests, two desktop E2E tests, one mobile E2E test, catalog validation,
and full specification lint. Changed-file ESLint and TypeScript also passed.

The CI failure reproduced on GitHub merge tree a9c2fb9506: its newer ACP SDK
discarded the unnamed mode selector in the test fixture. Adding the required
name made the focused test pass three times and the full utility package pass
under the race detector on that merge tree. A grouped absent-mode case also
confirms that the advertisement was read instead of falling through to apply.
The trusted-base PR documentation evaluator reports this package as covered.
Public documentation validation covers the profile guide update.
