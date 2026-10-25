---
id: "08-release-troubleshooting-docs"
title: "Document release troubleshooting"
status: pending
wave: 7
depends_on: ["05-responsive-mcp-status-surface", "06-acpdbg-sentinel-probe", "07-responsive-status-e2e"]
plan: "plan.md"
spec: "../../specs/platform/requirements/mcp-session-observability.md"
---

# Task 08: Document release troubleshooting

## Acceptance

- Public docs explain configured, filtered, delivered, connected, tools loaded,
  used, and failed without implying third-party connectivity is always
  observable.
- Release users can find the toolbar popover/drawer, endpoint test, recent
  output warning, sanitized copy, settings link, and reset-context consequence.
- Privacy copy states exactly what persists and what raw data stays
  development-only or ephemeral.
- Agent integration docs show the sentinel `acpdbg` command and require honest
  passthrough strategy reporting.
- Documentation uses the final shipped labels and screenshots only if stable
  assets are intentionally added.

## Verification

- `rtk rg -n "Delivered|connection unverified|mcp-probe|raw ACP" docs/public .agents/skills/acp-debug apps/backend/cmd/acpdbg/README.md`
- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`

## Files likely touched

- `docs/public/agents-and-profiles.md`
- `docs/public/automation-and-mcp.md`
- `docs/public/add-agent-cli.md`
- `docs/public/feature-status.md`
- `apps/backend/cmd/acpdbg/README.md`
- `.agents/skills/acp-debug/SKILL.md`

## Dependencies

- Task 05 fixes user-visible labels and actions.
- Task 06 fixes developer CLI syntax and interpretation.
- Task 07 verifies the final responsive flow.

## Parallelism

Sequential finalization. Writing public instructions before final labels and
verified behavior would create drift.

## Inputs

- Docs-maintainer boundaries and validation commands
- Final component labels and diagnostic behavior
- Final `acpdbg mcp-probe` CLI help and README

## Output contract

Report public docs changed, final terminology, privacy/uncertainty wording,
validation results, files changed, blockers, and risks. Mark this task `done`
and update its plan checkbox.
