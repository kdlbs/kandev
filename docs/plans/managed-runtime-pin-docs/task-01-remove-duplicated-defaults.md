---
id: "01-remove-duplicated-defaults"
title: "Remove duplicated runtime defaults"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-UPDATES-001
acceptance_criteria:
  - AC-AGENTS-RUNTIME-UPDATES-001.5
  - AC-AGENTS-RUNTIME-UPDATES-001.9
system_design:
  - ../../specs/agents/system-design/runtime-updates-01.md
---

# Task 01: Remove duplicated runtime defaults

## Summary

Make the JSON catalogue the reference for current runtime defaults in three
documents. Preserve exact historical versions and the existing runtime behavior.

## In scope

- Remove the default-version column from the ACP runtime table.
- Replace the concrete current Claude command with an explained effective-version placeholder.
- Replace the ADR's current Codex version with a catalogue link.
- Separate the timeout experiment's tested version from the current default.
- Use relative Markdown links to the JSON catalogue in all three documents.

## Out of scope

Runtime code, catalogue values, workflow changes, generated tables, permanent
test changes, public website changes, and GitHub mutations are excluded.

## Acceptance

1. All three documents link to the existing JSON catalogue for current defaults.
   The runtime table and launch example contain no duplicated default numbers.
2. The timeout document retains the exact tested versions, binary path, and
   experiment results. It makes no claim that those versions remain current.
3. Package names, ACP arguments, effective-version semantics, and the JSON-only
   maintenance path remain unchanged. All listed verification commands pass.

## Verification

Run all commands from the repository root.

### Current-default documentation audit

Before editing, run this search and record the stale claims:

```bash
rtk proxy rg -n -A 3 -B 2 'at this commit|currently|pinned at|Default version' apps/backend/internal/agent/agents/ACP_BRIDGE_VERSIONS.md docs/decisions/0034-agentclientprotocol-codex-acp.md docs/specs/agents/system-design/mcp-timeout-budgets.md
```

After editing, repeat the search. Inspect any remaining matches in context.
The timeout document can still describe unrelated settings as currently unset.
No match can assert that a literal runtime version is the current default.

Run this search to inspect the links and preserved experiment identifiers:

```bash
rtk proxy rg -n 'managed_npm_runtime_versions.json|0\.75\.1|0\.3\.257|effective-version' apps/backend/internal/agent/agents/ACP_BRIDGE_VERSIONS.md docs/decisions/0034-agentclientprotocol-codex-acp.md docs/specs/agents/system-design/mcp-timeout-budgets.md
```

Resolve each Markdown link relative to its document. All three must reach
`apps/backend/internal/agent/agents/managed_npm_runtime_versions.json`.
Inspect the diff to verify that experiment measurements and conclusions remain intact.

### Existing automated checks

```bash
rtk proxy node --test scripts/update-agent-runtime-pins.test.mjs
rtk proxy python3 .github/scripts/update-agent-runtime-pins-workflow-contract_test.py
rtk proxy python3 scripts/lint-spec-files.py --all
rtk proxy git diff --check
rtk proxy git diff --stat
rtk proxy git status --short
```

The final diff contains only the three documentation corrections and package
status updates. Record each result before marking this work order done.

## Files likely touched

- `apps/backend/internal/agent/agents/ACP_BRIDGE_VERSIONS.md`
- `docs/decisions/0034-agentclientprotocol-codex-acp.md`
- `docs/specs/agents/system-design/mcp-timeout-budgets.md`
- `docs/plans/managed-runtime-pin-docs/plan.md`
- `docs/plans/managed-runtime-pin-docs/task-01-remove-duplicated-defaults.md`

## Dependencies

None. The completed documentation repair must reach `main` before a scheduled
run can inherit it. Publication remains a separate delivery step.

## Risks

Global version replacements can corrupt historical evidence. A repair confined
to the bot branch can disappear on the next workflow run.

## Parallelism

`sequential`

## Inputs

- [Runtime requirements](../../specs/agents/requirements/runtime-updates.md), criteria 001.5 and 001.9.
- [Scheduled update design](../../specs/agents/system-design/runtime-updates-01.md#scheduled-pin-update-workflow).
- [Review evidence and technical approach](plan.md).
- `scripts/update-agent-runtime-pins.mjs` and its existing fixture tests.
- `.github/workflows/update-agent-runtime-pins.yml` and its existing contract tests.

## Results

Implemented the documentation repair. The ACP runtime table now links to the
JSON catalogue and contains no duplicated default versions. The Claude launch
example uses an effective-version placeholder with its exact-version meaning.
ADR 0034 links to the catalogue instead of naming a current Codex version.
The timeout design preserves the tested Claude and SDK versions, binary path,
measurements, and conclusions while identifying them as historical experiment
evidence.

Verification passed:

- `node --test scripts/update-agent-runtime-pins.test.mjs`: 7 tests passed.
- `python3 .github/scripts/update-agent-runtime-pins-workflow-contract_test.py`: 9 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `make -C apps/backend build`: passed. The build reported non-fatal missing
  `codesign`/`rcodesign` warnings for Darwin artifacts.
- The post-edit current-default documentation audit found no stale default
  claims. The remaining `currently` match describes an unrelated unset value.
- All three relative catalogue links resolve to the existing JSON catalogue.
- `git diff --check`: passed.
