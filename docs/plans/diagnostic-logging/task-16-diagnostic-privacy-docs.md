---
id: "16-diagnostic-privacy-docs"
title: "Diagnostic privacy documentation"
status: done
wave: 12
depends_on:
  - "14-bundle-customizer-ui"
plan: "plan.md"
spec: "../../specs/platform/requirements/diagnostic-logging.md"
---

# Task 16: Diagnostic privacy documentation

## Acceptance

- Public docs distinguish standard frontend/backend event evidence from stored
  message tables and truthfully warn that incidental emitted log text remains.
- Debug docs explain raw+normalized ACP contents, owner/admin session selection,
  debug capability gating, on-demand host/executor collection, fixed limits,
  partial unavailable sessions, runtime-index fields, and no continuous ACP
  centralization.
- Debug skills inspect standard sources first, treat ACP as explicit
  human-provided sensitive evidence, inspect `manifest.json`, and exact-grep
  task/session IDs without claiming ACP is message-free.

## Verification

```bash
rg -n "ACP|agent messages|session messages|runtime index|diagnostic bundle" \
  docs/public .agents/skills/debug
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
make lint-harness
```

## Files likely touched

- `docs/public/cli.md`
- `docs/public/configuration.md`
- `docs/public/docker.md`
- `docs/public/k8s.md`
- `docs/public/operations.md`
- `docs/public/remote-cloud-environment.md`
- `.agents/skills/debug/SKILL.md`
- `.agents/skills/debug/references/browser.md`
- `.agents/skills/debug/references/instance.md`

## Dependencies

- Task 14 settles shipped labels, capability behavior, limits, and partial UI
  wording before public and agent guidance is finalized.

## Parallelism

Parallel-safe with Task 15 after Task 14. It owns docs/skill files only.

## Inputs

- Spec complete privacy, source, permission, failure, persistence, and scenario
  contracts.
- ADR consent/on-demand decisions and rejected alternatives.
- Docs-maintainer and debug-skill conventions.

## Risks

- “Does not read stored messages” must not be shortened into “contains no
  messages”; frontend/backend logs can carry incidental emitted content, and
  ACP deliberately carries message/tool/file payloads.
- Existing remote-cloud wording currently says to exclude ACP from support
  bundles; update it to distinguish standard bundles from explicit selected
  ACP debug bundles rather than deleting the privacy warning.

## Output contract

Report docs/skill changes, stale claims removed, exact validators/results,
blockers/risks, and synchronize this task plus `plan.md` status.

## Results

Updated public Logs guidance, the remote-cloud ACP warning, and debug skill
references with source boundaries, event classes, ACP sensitivity, fixed
budgets, partial behavior, manifest-first inspection, and exact task/session
grep commands. Standard evidence is explicitly distinguished from stored
messages and from opt-in ACP protocol content.

- Verification: `node --test scripts/validate-public-docs.test.mjs` (58 passed).
- Verification: `node scripts/validate-public-docs.mjs` (41 docs validated).
- Verification: `make lint-harness` (112 harness files passed).
