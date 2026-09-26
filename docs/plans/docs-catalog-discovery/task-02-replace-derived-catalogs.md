---
id: "02-replace-derived-catalogs"
title: "Replace tracked derived catalogs"
status: done
wave: 2
depends_on: ["01-build-catalog-command"]
plan: "plan.md"
requirements: []
acceptance_criteria: []
system_design: []
---

# Task 02: Replace tracked derived catalogs

## Summary

Remove tracked lists that the catalog command can derive. Keep static entry
pages and durable system ownership text.

## In scope

- Replace `docs/decisions/INDEX.md` with a short decision-log entry page.
- Replace `docs/specs/INDEX.md` with a short specification-catalog entry page.
- Remove exhaustive specification maps from every system README.
- Keep each system README purpose, scope, ownership, exclusions, migration
  record, and related-system links.
- Add concise command examples for common catalog searches.
- Update specification guides and the system README template.
- Mark ADR-0001 and ADR-2026-08-22-system-oriented-specifications as amended by
  the catalog decision.
- Repair links that target removed table rows or map sections.
- Replace the product README's per-file document list with a catalog command.
- Audit active specification sources for references to removed index rows and
  stale "above" list wording.

## Out of scope

- Rewriting requirements or system designs.
- Changing public documentation navigation or coverage data.
- Removing curated prose from system README files.

## Implementation details

Do not commit command output. The two index files explain how to find current
documents with `scripts/list-docs.py`.

Identify system README files from their frontmatter, not from a fixed directory
list. Remove only sections that enumerate requirement or system-design files.
Preserve migration history and boundary statements even when those sections
contain links.

Update these authoring sources where they describe tracked indexes or maps:

- `docs/specs/README.md`
- `docs/specs/guide/structure-and-ownership.md`
- `docs/specs/guide/requirements.md`
- `docs/specs/guide/traceability-and-lifecycle.md`
- `docs/specs/templates/system-readme.md`

## Acceptance

- Adding one ADR does not require an edit to any existing documentation file.
- Adding one requirement or system design does not require an index or map edit.
- Both static index pages show working list and filter examples.
- Every system README retains its durable ownership and migration information.
- No repository guidance points authors to a removed table or map.
- The real repository passes catalog and specification validation.

## Verification

```bash
python3 scripts/list-docs.py validate
python3 scripts/list-docs.py decisions --format markdown
python3 scripts/list-docs.py specs --format markdown
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
rg -n "Specification map|add.*INDEX|update.*INDEX" docs AGENTS.md .agents
! rg -n -i "docs/specs/INDEX\.md.*(row|status)|INDEX\.md.*row|the (seven|[0-9]+|current|listed) requirements? above|the (seven|[0-9]+|current|listed) system[- ]designs? above" docs/specs AGENTS.md .agents
git diff --check -- docs/decisions docs/specs docs/plans
```

## Files likely touched

- `docs/decisions/INDEX.md`
- `docs/decisions/0001-file-based-knowledge-system.md`
- `docs/decisions/2026-08-22-system-oriented-specifications.md`
- `docs/specs/INDEX.md`
- `docs/specs/README.md`
- `docs/specs/*/README.md`
- `docs/specs/guide/*.md`
- `docs/specs/templates/system-readme.md`

## Dependencies

- Task 01 supplies the command referenced by the static pages.

## Risks

- A broad README edit can remove curated context. Review each changed section
  and keep all non-derived content.
- Old links can still resolve while pointing to obsolete instructions. Search
  both link targets and authoring language.

## Parallelism

`sequential`

## Inputs

- `docs/decisions/2026-09-07-on-demand-document-catalogs.md`
- `scripts/list-docs.py`
- `docs/decisions/INDEX.md`
- `docs/specs/INDEX.md`
- `docs/specs/*/README.md`

## Results

Done. Both root catalog pages are static entry pages, all derived system
README maps and the product per-file list are removed, and active source
references use on-demand discovery. The specification and catalog validation
checks pass.
