---
id: "06-debug-workflow-guidance"
title: "Debug workflow guidance"
status: completed
wave: 7
depends_on:
  - "09-remove-legacy-diagnostics"
  - "10-agent-diagnostic-materialization"
plan: "plan.md"
spec: "../../specs/platform/diagnostic-logging.md"
---

# Task 06: Debug workflow guidance

## Acceptance

- Task-session debug uses `get_diagnostic_bundle_kandev` and requests backend,
  frontend, or all based on the bug class instead of calling the removed debug
  export or tail.
- The workflow creates a fresh temporary directory, downloads the selected ZIP,
  extracts it safely without overwriting an existing path, and inspects
  `manifest.json` before trusting source completeness.
- When a task ID is known, the workflow searches it exactly with
  `rg --fixed-strings` before broadening to session ID, route/error text, or a
  bounded time window.
- A zero-match task-ID search is described as inconclusive because global logs
  and frontend pages outside recognized task routes may omit `task_id`.
- Live-instance safety remains unchanged: bundle creation/download is read-only,
  agents do not relaunch the user's instance, and they remove only temporary
  extraction directories they created.
- Host-side `scripts/kandev-logs` works without credentials only when auth is
  disabled; authenticated use reads a PAT from `KANDEV_API_TOKEN` and never
  accepts or prints it as an argument.

## Verification

```bash
git diff --check -- .agents/skills/debug scripts/kandev-logs
```

```bash
python3 scripts/lint-harness-files.test.py
```

```bash
scripts/kandev-logs --help
```

```bash
rg -n "bundle|manifest|backend|frontend|task ID|task_id|fixed-strings" \
  .agents/skills/debug/SKILL.md \
  .agents/skills/debug/references/instance.md \
  .agents/skills/debug/references/browser.md \
  scripts/kandev-logs
```

## Files likely touched

- `.agents/skills/debug/SKILL.md`
- `.agents/skills/debug/references/instance.md`
- `.agents/skills/debug/references/browser.md`
- `scripts/kandev-logs`
- focused shell tests for `scripts/kandev-logs`

## Dependencies

- Task 09 removes legacy diagnostics.
- Task 10 provides owner-scoped MCP materialization and the authenticated
  host-helper contract.

## Parallelism

Parallel-safe with Task 05 after Tasks 09 and 10. It owns agent-harness files
and the log helper; Task 05 owns public documentation.

## Inputs

- Spec: Agent diagnostics, bundle API, partial-bundle behavior, and scenarios.
- Plan: Debug harness.
- Harness-improvement skill: update the narrowest existing skill and preserve
  progressive disclosure.
- Existing patterns: debug triage gate, live-instance safety,
  `kandev-instances`, `kandev-logs`, and browser/backend correlation.

## Risks

- Agents must inspect `manifest.json`; an all-sources or frontend bundle can be
  partial when no browser responds.
- Extraction must reject traversal and must never target a workspace,
  repository root, home directory, or reused broad path.
- Task IDs are correlation hints, not authorization or complete trace IDs.
- The helper must work for default and isolated instances and must not hard-code
  `~/.kandev`.

## Output contract

Report source-selection guidance, extraction safety, exact evidence order,
files changed, validation commands/results, remaining correlation gaps, and
update this task plus `plan.md` status in the same conversation.
