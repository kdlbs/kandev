---
id: "01-validate-evidence"
title: "Add the screenshot presence check"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-CI-UI-SCREENSHOTS-001
  - REQ-CI-UI-SCREENSHOTS-002
  - REQ-CI-UI-SCREENSHOTS-003
acceptance_criteria:
  - AC-CI-UI-SCREENSHOTS-001.1
  - AC-CI-UI-SCREENSHOTS-001.2
  - AC-CI-UI-SCREENSHOTS-001.3
  - AC-CI-UI-SCREENSHOTS-001.4
  - AC-CI-UI-SCREENSHOTS-001.5
  - AC-CI-UI-SCREENSHOTS-001.6
  - AC-CI-UI-SCREENSHOTS-001.7
  - AC-CI-UI-SCREENSHOTS-001.8
  - AC-CI-UI-SCREENSHOTS-002.1
  - AC-CI-UI-SCREENSHOTS-002.2
  - AC-CI-UI-SCREENSHOTS-002.3
  - AC-CI-UI-SCREENSHOTS-002.4
  - AC-CI-UI-SCREENSHOTS-003.1
  - AC-CI-UI-SCREENSHOTS-003.2
  - AC-CI-UI-SCREENSHOTS-003.3
  - AC-CI-UI-SCREENSHOTS-003.6
system_design:
  - ../../specs/ci/system-design/ui-screenshot-evidence.md
---

# Task 01: Add the screenshot presence check

## Summary

Add one metadata-only check and matching author instructions.
A UI candidate PR passes with a screenshot embed or the explicit maintainer exception label.

## In scope

- Add the small path classifier, Screenshots-section parser, and status adapter.
- Reuse the existing GitHub client, label-exception pattern, and trusted workflow structure.
- Add focused Node and workflow contract tests.
- Update the PR skill, PR template, CONTRIBUTING.md, and existing public contributor guide together.

## Out of scope

Image downloads, commit declarations, custom comment commands, merge queues, new dependencies, and visual approval.

## Acceptance

1. UI without an image fails. UI with an HTTPS Markdown image in Screenshots passes. Unrelated badges and author self-waivers do not pass.
2. Non-UI changes pass. The exact label bypasses this check, and label removal restores normal evaluation. Forks follow the same rule.
3. Tests prove complete metadata reads, current-head status publication, correct events, least privilege, and no fork execution. Instructions match the rule.

## Verification

Run from the repository root:

```bash
node --test .github/scripts/pr-ui-evidence.test.cjs
python3 .github/scripts/pr-ui-evidence-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
zizmor .github/workflows
python3 scripts/lint-harness-files.test.py
python3 .github/scripts/lint-harness-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `.github/scripts/pr-ui-evidence.cjs` and its Node test (new).
- `.github/workflows/pr-ui-evidence.yml` and its workflow contract test (new).
- `.github/workflows/lint-action-pinning.yml`.
- `.agents/skills/pr/SKILL.md`.
- `.github/pull_request_template.md`.
- `CONTRIBUTING.md` and `docs/public/contributing.md`.

## Dependencies

None.

## Risks

Presence is not visual proof. Explain the distinction in the result and contributor instructions.
Keep source code at the trusted revision for fork PRs.

## Parallelism

`sequential`

## Inputs

Read the screenshot requirement/design, `.github/AGENTS.md`, and the existing `pr-docs.yml` and `pr-docs.cjs` patterns.

## Results

Completed. Added a metadata-only `UI screenshots` status. Runtime candidates
under web, shared UI, and desktop paths require an HTTPS Markdown image with
nonempty alt text in the `## Screenshots` section. Test, documentation, script,
lockfile, translation-catalog, and tool-configuration paths are excluded.
The exact `no-screenshots-allow` label is the only exception. Both PRs from
forks and repository branches use the same rule. The check rereads PR metadata,
retries changed snapshots, reads the complete API file list, and publishes the
status to the evaluated head without checking out or executing PR code.

Updated the PR skill, PR template, root contribution guide, and public
contributor guide. They distinguish screenshot presence from reviewer judgment
of freshness, visibility, relevance, and viewport coverage.

Verification passed:

- `node --test .github/scripts/pr-ui-evidence.test.cjs` (13 tests)
- `python3 .github/scripts/pr-ui-evidence-workflow-contract_test.py` (7 tests)
- `python3 .github/scripts/lint-action-pinning_test.py`
- `python3 .github/scripts/lint-action-pinning.py`
- `zizmor .github/workflows/pr-ui-evidence.yml` (no findings)
- `python3 scripts/lint-harness-files.test.py` (19 tests)
- `python3 .github/scripts/lint-harness-files.py --all` (205 files)
- `node --test scripts/validate-public-docs.test.mjs` (63 tests)
- `node scripts/validate-public-docs.mjs` (55 pages)
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`

`zizmor .github/workflows` reported existing findings in unrelated workflows
(`backend-tests.yml`, `claude-code-review.yml`, `isolated-windows-tests.yml`,
`opencode-code-review.yml`, `pr-walkthrough-reconcile.yml`,
`pr-walkthrough.yml`, `preview-env.yml`, and `revoke-ready-to-merge.yml`). The
new workflow itself passes the audit. The hosted required-check rollout remains
pending until this implementation is merged and an administrator authorizes
the live ruleset and smoke checks.

Review follow-up keeps all triggered PR events in the concurrency queue with
`queue: max`, so a later filtered event cannot replace a pending body or
exception-label reevaluation. The parser rejects escaped image syntax and
four-space-indented code examples. Translation exclusions are limited to
locale catalog paths and `.po`/`.pot` catalogs; runtime files under `lib/i18n/`
remain screenshot candidates.

Follow-up verification passed:

- `node --test .github/scripts/pr-ui-evidence.test.cjs` (10 tests)
- `python3 .github/scripts/pr-ui-evidence-workflow-contract_test.py` (7 tests)
- `python3 .github/scripts/lint-action-pinning_test.py`
- `python3 .github/scripts/lint-action-pinning.py`
- `zizmor .github/workflows/pr-ui-evidence.yml` (no findings)
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`

Current-head review follow-up also includes `.jsx` components as UI candidates,
lists safely escaped candidate paths when evidence is missing, and ignores
matched inline code spans that cross line endings. It retains the escaped-image
and indented-code exclusions. The system design now records the Markdown forms
that the presence check ignores.

PR fixup verification passed:

- `node --test .github/scripts/pr-ui-evidence.test.cjs` (13 tests)
- `python3 .github/scripts/pr-ui-evidence-workflow-contract_test.py` (7 tests)
- `git diff --check`
