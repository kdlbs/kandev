---
id: "05-contracts-and-rollout"
title: "Reconcile contracts and document rollout"
status: completed
wave: 5
depends_on:
  - "04-recovery-proof"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-001
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-002
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-003
acceptance_criteria:
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.1
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.2
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.3
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.4
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.5
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.2
system_design:
  - ../../specs/platform/system-design/provider-interruption-continuation.md
---

# Task 05: Reconcile contracts and document rollout

## Summary

Document the implemented recovery boundary and how operators enable and disable
it on a selected installation. Reconcile the prior pre-result-only wording
without weakening the replay, dynamic-routing, or unknown-effect contracts.

## In scope

- Use docs-maintainer to update `docs/public/sessions-and-review.md` and the
  owning configuration/feature-status sections found by catalog/search. Explain
  preserved history, continuation versus replay, five shared attempts, Cancel,
  restart behavior, manual writes/uncertain tools, and truthful exhaustion.
- State the exact flag key/environment, all-off shipped defaults, existing
  environment/SQLite/profile precedence, restart requirement, selected-install
  enablement, and disable/rollback procedure. No default-on promise.
- Replace broad automatic-recovery wording in existing provider-error recovery
  criteria `.8`/`.15` with a concise reference to the separate continuation
  contract; keep original replay and dynamic/Office boundaries unchanged.
  Amend related design/ADR references and scoped AGENTS.md only where changed
  contracts make them inaccurate.
- Existing provider-error requirement/design are within roughly 100 bytes of
  limits. Reduce/replace prose or split by contract boundary; never raise limits.
- Record native compatibility evidence and every prior work-order result. Promote
  paired draft docs to active/current and proposed ADR to accepted only after
  actual implementation agrees and all task-defined checks pass.

## Out of scope

Publishing docs, committing, pushing, PR creation, enabling the user's live
installation, automatic release-toggle promotion, or a broad local test audit.

## Acceptance

- Public guidance describes the shipped off-by-default behavior and exact
  selected-install enable/disable steps without implying write recovery or
  exactly-once execution.
- Prior and new contracts agree on replay versus continuation; native evidence
  and task results support every claimed capability, with unsupported shapes
  and remaining risks explicitly retained.
- Catalog, specification lint, public-doc validation, and documentation-reference
  coverage preflight pass; plan/task lifecycle status reflects actual delivery.

## Verification

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node scripts/validate-public-docs.mjs
git diff --check -- docs/specs docs/decisions docs/plans/provider-interruption-continuation docs/public
git status --short
```

Run local `validateCoverage` from `.github/scripts/pr-docs.cjs` against changed
work orders and their actual file contents, including all referenced designs,
requirements, and the plan. For a design-only diff, use a clearly reported
synthetic existing backend trigger to force reference validation; do not claim
it is a production change. After implementation use actual changed paths.
No GitHub publication/status write is necessary for this local preflight.

Repeatable implementation-package reference check from repo root:

```bash
node <<'JS'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const directory = 'docs/plans/provider-interruption-continuation';
const docs = fs.readdirSync(directory)
  .filter(name => name.endsWith('.md'))
  .map(name => `${directory}/${name}`);
const references = [
  'docs/specs/platform/requirements/provider-interruption-continuation.md',
  'docs/specs/platform/system-design/provider-interruption-continuation.md',
];
const fileContents = Object.fromEntries([...docs, ...references]
  .map(filename => [filename, fs.readFileSync(filename, 'utf8')]));
// The implemented package changes this existing backend path.
const changedFiles = [...docs,
  'apps/backend/internal/orchestrator/event_handlers_transient.go'];
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({ ok: result.ok, errors: result.errors,
  workOrders: result.workOrders.length }, null, 2));
if (!result.ok) process.exitCode = 1;
JS
```

## Files likely touched

- `docs/public/sessions-and-review.md`
- Existing public configuration and `docs/public/feature-status.md` sections
- `docs/specs/platform/requirements/provider-error-recovery.md`
- `docs/specs/platform/system-design/provider-error-recovery.md`
- Paired continuation requirement/design and accepted ADR
- `docs/plans/provider-interruption-continuation/plan.md`, work-order results,
  and `compatibility-evidence.md`
- Scoped backend/agentctl/web AGENTS.md if new evidence/restore boundaries need guidance

## Dependencies

Tasks 01-04 complete. Do not promote unsupported or unresolved native contracts.

## Risks

Documentation can silently promise replay after writes or default-on recovery.
Check introductory summaries against precise criteria and toggle defaults.

## Parallelism

`sequential`

## Inputs

Paired requirements/design, proposed ADR, native compatibility results, task
validation results, docs-maintainer, runtime-feature-flags, and spec lifecycle guide.

## Results

Public recovery, configuration, and feature-status guidance documents the
experimental default-off toggle, selected-install enable/disable and restart,
same-ID saved-history continuation, safe re-reading, shared budget, cancellation,
and manual unsafe work. Prior provider-error replay criteria and design now
reference the separate continuation boundary. Catalog validation, all-spec
lint, and public-doc validation pass. Actual changed-file coverage validates all six work orders and their linked
contracts. Requirements are active, the design is current, the decision is
accepted, and all work orders are complete. The toggle remains off and changes
are prepared for publication; no deployment or live-session mutation was performed.

Physical phone testing was unavailable. Phone results use Pixel 5 emulation;
no physical-device validation or associated issue closure is claimed.

PR review remediation reconciles continuation eligibility and the prior provider-guaranteed replay exception, corrects native probe evidence, and makes backend/browser commands reproducible. Catalog, specification, public-doc, harness, and changed-file documentation coverage checks pass. Phone coverage uses browser emulation; no physical device or real Wi-Fi switch is claimed.
