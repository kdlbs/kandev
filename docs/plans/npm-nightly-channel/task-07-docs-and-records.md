---
id: "07-docs-and-records"
title: "Public docs and final records"
status: completed
wave: 6
depends_on: ["02-release-workflow", "04-backend-api-apply", "06-updates-channel-e2e"]
plan: "plan.md"
spec: "../../specs/release/requirements/npm-nightly-channel.md"
---

# Task 07: Public docs and final records

- **Acceptance:** install, service, operations, and release docs describe `@nightly`, Stable default,
  supported channel switching, exact noon best-effort cadence, recovery, and exclusions.
- **Acceptance:** spec, ADR, plan, and task statuses match the implemented behavior and recorded
  command results.
- **Acceptance:** public documentation validation and diff whitespace checks pass.
- **Verification:** `node --test scripts/validate-public-docs.test.mjs`
- **Verification:** `node scripts/validate-public-docs.mjs`
- **Verification:** `git diff --check`
- **Files likely touched:** `README.md`, `docs/public/cli.md`, `use-kandev.md`, `operations.md`,
  `run-as-a-service.md`, `release-process.md`, `AGENTS.md`, `.agents/skills/release/SKILL.md`, and
  this feature's durable records.
- **Dependencies:** Tasks 02, 04, and 06.
- **Parallelism:** sequential.
- **Inputs:** landed behavior and `/docs-maintainer` task-oriented documentation rules.
- **Risks:** docs must not imply Homebrew/Desktop/GHCR nightly support or exact GitHub cron start.

## Verification results

- `node --test scripts/validate-public-docs.test.mjs` — passed, 58 tests.
- `node scripts/validate-public-docs.mjs` — passed, 41 published pages validated.
- `git diff --check` — passed.
