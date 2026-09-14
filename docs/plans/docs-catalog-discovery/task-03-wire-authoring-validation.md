---
id: "03-wire-authoring-validation"
title: "Wire authoring guidance and validation"
status: done
wave: 3
depends_on: ["02-replace-derived-catalogs"]
plan: "plan.md"
requirements: []
acceptance_criteria: []
system_design: []
---

# Task 03: Wire authoring guidance and validation

## Summary

Make on-demand discovery the repository authoring rule. Run validation in local
hooks and CI without generating or changing files.

## In scope

- Update root agent guidance to use the catalog command.
- Update the record, specification, planning, fix, and context-engineering
  skills where they require tracked index edits or reads.
- Add a `docs-catalog` pre-commit hook that runs repository validation.
- Run catalog tests and validation in the existing harness lint workflow.
- Confirm that normal ADR and specification additions do not modify shared
  catalog files.

## Out of scope

- Automatic file generation or formatting.
- A bot, scheduled job, or post-merge workflow.
- Changes to unrelated skill behavior.

## Implementation details

The pre-commit hook uses `python3 scripts/list-docs.py validate`. Configure its
file match for decisions, specifications, the catalog command, the shared
metadata helper, and the specification linter. It must not pass filenames
because validation checks cross-file identities.

The CI workflow first runs `scripts/list-docs.test.py`, then validates the real
repository. Keep the checks in the existing lightweight harness lint job.

Update instructions to distinguish discovery from durable navigation. Authors
use the command for ADR and specification lists. They continue to edit curated
public navigation and coverage files when those contracts change.

## Acceptance

- Pre-commit rejects invalid catalog metadata and never rewrites files.
- Changes to the specification linter or shared metadata helper run catalog
  validation as well.
- CI runs focused catalog tests and full repository validation.
- Agent guidance no longer tells authors to add decision rows or specification
  map entries.
- Planning and context skills use filtered catalog commands for discovery.
- Public navigation and coverage guidance remains manual and explicit.
- Harness, specification, and catalog checks all pass.

## Verification

```bash
python3 scripts/list-docs.test.py
python3 scripts/list-docs.py validate
python3 scripts/lint-harness-files.test.py
python3 .github/scripts/lint-harness-files.py --all
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
pre-commit run docs-catalog --all-files
git diff --check -- .pre-commit-config.yaml .github AGENTS.md .agents docs
```

## Files likely touched

- `AGENTS.md`
- `.agents/skills/record/SKILL.md`
- `.agents/skills/spec/SKILL.md`
- `.agents/skills/plan/SKILL.md`
- `.agents/skills/fix/SKILL.md`
- `.agents/skills/context-engineering/SKILL.md`
- `.pre-commit-config.yaml`
- `.github/workflows/lint-harness-files.yml`
- `scripts/spec_metadata.py`

## Dependencies

- Task 01 supplies validation.
- Task 02 establishes the static documentation model.

## Risks

- One stale skill can recreate shared-file edits. Search all harness files for
  index and specification-map instructions.
- A narrow hook pattern can miss new document paths. Include both complete
  documentation trees and the command files.

## Parallelism

`sequential`

## Inputs

- `docs/decisions/2026-09-07-on-demand-document-catalogs.md`
- `scripts/list-docs.py`
- `.pre-commit-config.yaml`
- `.github/workflows/lint-harness-files.yml`

## Results

Done. Agent guidance, skills, pre-commit, and harness CI use read-only catalog
validation. The specification skill includes the catalog check. Harness,
specification, catalog, reference-audit, and whitespace checks pass.
