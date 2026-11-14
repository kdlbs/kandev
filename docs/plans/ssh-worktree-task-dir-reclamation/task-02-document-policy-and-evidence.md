---
id: "02-document-policy-and-evidence"
title: "Document SSH reclamation policy and evidence"
status: done
wave: 2
depends_on:
  - "01-correct-checkout-scoped-reclamation"
plan: "plan.md"
requirements:
  - REQ-SSH-TASKDIR-RECLAMATION-001
  - REQ-SSH-TASKDIR-RECLAMATION-002
  - REQ-SSH-TASKDIR-RECLAMATION-006
acceptance_criteria:
  - AC-SSH-TASKDIR-RECLAMATION-001.5
  - AC-SSH-TASKDIR-RECLAMATION-002.3
  - AC-SSH-TASKDIR-RECLAMATION-002.4
  - AC-SSH-TASKDIR-RECLAMATION-002.8
  - AC-SSH-TASKDIR-RECLAMATION-002.9
  - AC-SSH-TASKDIR-RECLAMATION-002.10
  - AC-SSH-TASKDIR-RECLAMATION-002.11
  - AC-SSH-TASKDIR-RECLAMATION-002.12
  - AC-SSH-TASKDIR-RECLAMATION-006.1
  - AC-SSH-TASKDIR-RECLAMATION-006.2
system_design:
  - ../../specs/executors/system-design/remote-task-directory-reclamation.md
---

# Task 02: Document SSH Reclamation Policy and Evidence

## Summary

Publish the implemented safety policy in the existing SSH reference sections.
Reconcile the requirements, system design, and delivery results with task 01's
actual behavior and recorded command results.

## In scope

- Use `/docs-maintainer` and the public documentation guide.
- Update the SSH feature-status row and the executor page's cleanup section.
- State the changed ignored-file policy, including ignored secrets and existing
  enabled profiles. Keep this eligibility information visible outside disclosures.
- Explain task-HEAD and recorded-branch stash checks for retained repositories,
  repository-wide checks for standalone clones, and unknown-state preservation.
- Preserve the opt-in default, terminal-only trigger, and existing archive behavior.
- Distinguish default standalone preparation from custom or reused linked worktrees.
- State cleanup's required Git capabilities and preservation of known locks and
  submodule layouts. Do not imply that these limits change SSH launch support.
- State that existing orphan directories receive no sweep.
- Search the same policy across public docs, specs, decisions, the root README,
  and the screenshot catalog. Correct affected current guidance only.
- Retain the original completed package as history with its follow-up link.
- Record every required check in both work orders and the manifest.

## Out of scope

- Product code, new controls, screenshots, localization catalogs, or unrelated docs.
- Rewriting historical command results as if they tested the new implementation.
- Commit, push, or PR without the user's separate authorization.

## Acceptance

1. Public reference pages describe the implemented scope and ignored-file effect,
   without promising reclamation for ambiguous layouts or historical orphans.
2. Specification lint, public-doc validation, and the existing PR-documentation
   coverage helper pass for this package.
3. Both work orders and the manifest contain actual command results. Task 01's
   regressions and failure cases pass before the plan becomes `implemented`.

## Verification

Run this block from the repository root. The coverage preflight evaluates the
planned source path and both work orders against the local documents.
It does not publish a GitHub status.

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const dir = 'docs/plans/ssh-worktree-task-dir-reclamation';
const workOrders = fs.readdirSync(dir).filter(p => /^task-\d{2}-.*\.md$/.test(p)).map(p => `${dir}/${p}`);
const paths = [
  `${dir}/plan.md`, ...workOrders,
  'docs/specs/executors/requirements/remote-task-directory-reclamation.md',
  'docs/specs/executors/system-design/remote-task-directory-reclamation.md',
];
const result = validateCoverage({
  changedFiles: [
    { filename: 'apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reclaim.go', status: 'modified' },
    ...workOrders.map(filename => ({ filename, status: 'added' })),
  ],
  fileContents: Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(JSON.stringify({ ok: result.ok, status: result.status, workOrders: result.workOrders, errors: result.errors }, null, 2));
if (!result.ok || result.workOrders.length !== 2) process.exitCode = 1;
NODE
git diff --check -- docs/specs docs/decisions docs/plans/ssh-worktree-task-dir-reclamation docs/plans/ssh-remote-task-dir-reclamation/plan.md docs/public
git status --short -- docs/plans/ssh-worktree-task-dir-reclamation
```

## Files likely touched

- `docs/public/feature-status.md`
- `docs/public/executors.md`
- `docs/specs/executors/requirements/remote-task-directory-reclamation.md`
- `docs/specs/executors/system-design/remote-task-directory-reclamation.md`
- `docs/plans/ssh-worktree-task-dir-reclamation/plan.md`
- This work order and task 01's **Results** section.

## Dependencies

Task 01 must be `done` with its targeted checks recorded.

## Risks

The current public row explicitly promises preservation of ignored files.
Publishing a replacement before implementation makes the current guide inaccurate.
Keep public edits in this work order, after the backend change passes.

## Parallelism

`sequential`

## Inputs

- Task 01 results and the plan's **Public documentation** section.
- Owning requirements: **Compatibility** and the listed acceptance criteria.
- Owning system design and the storage-scope ADR.
- `docs/public/README.md` and the original completed reclamation package.

## Results

Completed on 2026-11-14. Public SSH guidance now states the opt-in and
terminal-only behavior, the loss of ignored content (including secrets), the
different linked-worktree and standalone-clone checks, preservation limits, the
Git capability requirement, and the absence of an orphan sweep. The requirements,
system design, and implementation results match the delivered behavior. The
historical package remains linked as the earlier record.

Checks passed:

- `python3 scripts/list-docs.py validate` — validated 451 decisions and 1995 specifications.
- `python3 scripts/lint-spec-files.test.py` — 36 tests passed.
- `python3 scripts/lint-spec-files.py --all` — all specification files passed.
- `node --test scripts/validate-public-docs.test.mjs` — 63 tests passed.
- `node scripts/validate-public-docs.mjs` — validated 55 published documentation pages.
- The local `.github/scripts/pr-docs.cjs` coverage preflight — `covered`, both work orders accepted, zero errors.
- Policy search across the root README, public docs, executor specs, decisions, and screenshot catalog found no stale reclamation guidance in the README or screenshot catalog; other matches were current policy or unrelated SSH behavior.
- `git diff --check -- docs/specs docs/decisions docs/plans/ssh-worktree-task-dir-reclamation docs/plans/ssh-remote-task-dir-reclamation/plan.md docs/public` — passed.
