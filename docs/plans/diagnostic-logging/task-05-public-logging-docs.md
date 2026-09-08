---
id: "05-public-logging-docs"
title: "Public logging documentation"
status: completed
wave: 7
depends_on:
  - "01-backend-log-sinks"
  - "02-frontend-error-endpoint"
  - "03-launcher-contracts"
  - "04-toast-reporting"
  - "07-diagnostic-bundle-backend"
  - "08-browser-logs-and-bundle-ui"
  - "09-remove-legacy-diagnostics"
  - "10-agent-diagnostic-materialization"
plan: "plan.md"
spec: "../../specs/platform/requirements/diagnostic-logging.md"
---

# Task 05: Public logging documentation

## Acceptance

- Public configuration docs remove the destination and lumberjack rotation
  settings and accurately explain the fixed active file, UTC daily rollover,
  same-day restart append, rolling three-day retention, and file/stdout
  threshold matrix.
- Development and Docker docs identify the actual resolved log path and warn
  that bundles contain install-wide backend evidence plus sensitive browser
  URLs, console arguments, and stacks.
- Public docs explain source-selectable agent bundles, browser-requested
  capture, three-day local frontend retention, partial manifests, and the lack
  of continuous frontend upload.
- Public docs explain best-effort queue loss, bundle byte/profile truncation,
  256 MiB archive bounds, 15-minute ready lease, and `429`/`503` busy behavior.
- Public-doc validation passes with no stale examples of custom backend output
  destinations, individual System Logs downloads, tails, or debug exports.

## Verification

```bash
rg -n "logging\\.(outputPath|maxSizeMb|maxBackups|maxAgeDays|compress)|KANDEV_LOGGING_(OUTPUTPATH|MAXSIZEMB|MAXBACKUPS|MAXAGEDAYS|COMPRESS)|backend-logs\\.log|debug/export|logs/tail|diagnostic bundle" docs/public
```

```bash
node --test scripts/validate-public-docs.test.mjs
```

```bash
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `docs/public/configuration.md`
- `docs/public/contributing.md`
- `docs/public/docker.md`
- `docs/public/operations.md`
- `docs/public/k8s.md`

## Dependencies

Tasks 01–04 and 07–09 must settle the exact shipped behavior before public
wording is finalized.

## Parallelism

Parallel-safe with Task 06 after Tasks 09 and 10. It owns public documentation;
Task 06 owns agent-harness files and the log helper script.

## Inputs

- Spec: complete observable contract.
- ADR: consequences and rejected compatibility alternatives.
- Plan: Public documentation.
- Docs-maintainer skill requirements.

## Risks

- `logging.level` must be described as the file threshold, not a universal
  sink level.
- “Three days” must consistently mean the current UTC calendar day plus the
  two preceding UTC calendar days.
- Docker `/data` and development `.kandev-dev` examples must use their resolved
  homes rather than hard-coded `~/.kandev`.
- Docs must distinguish the backend's three on-disk days from frontend history,
  which stays local to a browser until a bundle requests it.

## Output contract

Report public docs changed, exact validation commands/results, stale references
found or removed, blockers or risks, and update this task plus `plan.md` status
in the same conversation.
