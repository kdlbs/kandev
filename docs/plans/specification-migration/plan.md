---
created: 2026-08-23
status: completed
requirements: []
system_design: []
---

# Implementation Plan: Specification migration

## Overview

Migrate all editable specifications from the pre-system layout into
system-owned requirements and system-design documents. The task system from PR
#2957 and the residual task-owned sources are included in the completed result.
Behavior is unchanged while ownership and traceability are explicit.

## Scope

### In scope

- Define system ownership for every remaining specification source.
- Create system indexes with migration state and authoritative specification
  maps.
- Extract stable `REQ-*` and `AC-*` identifiers from each capability and move
  technical contracts into paired system-design documents when needed.
- Update plans, decisions, indexes, and relative links to the new paths.
- Remove obsolete legacy files and size exceptions after each system is complete.

### Out of scope

- Runtime code or product behavior changes.
- Changes to the completed `docs/specs/tasks/` source documents except for
  links that point at migrated systems.
- New product capabilities or changes to existing contracts.

## Technical approach

Each source was assigned to one owning system. Observable requirements are
separated from technical design, large sources are split into bounded design
parts, and source detail is retained in the canonical documents. System
README migration states are `complete` because no editable legacy source
remains.

Ownership groups are: agents; auth; costs; integrations (including GitLab and
provider-specific root specs); office; platform; plugins; system-page;
workspaces; UI; and dedicated desktop, CLI, executor, and release/runtime
systems for the remaining standalone sources. Cross-cutting sources must link
to the owning system instead of duplicating its contract.

## Tests

This documentation migration has no runtime test changes. Structural evidence
is supplied by the specification linter, duplicate-ID checks, and the
relative-link/reference audit in the final work order.

## E2E tests

Not applicable. No user-visible behavior changes.

## Work orders

- [x] [Task 01: Establish ownership map](task-01-establish-ownership-map.md)
- [x] [Task 02: Migrate core systems](task-02-migrate-core-systems.md)
- [x] [Task 03: Migrate integrations and Office](task-03-migrate-integrations-office.md)
- [x] [Task 04: Migrate platform systems](task-04-migrate-platform-systems.md)
- [x] [Task 05: Migrate plugins and runtimes](task-05-migrate-plugins-runtimes.md)
- [x] [Task 06: Migrate UI and standalone sources](task-06-migrate-ui-standalone.md)
- [x] [Task 07: Retire legacy references](task-07-retire-legacy-references.md)
- [x] [Task 08: Verify complete migration](task-08-verify-complete-migration.md)

## Verification results

Completed on 2026-08-23:

- `python3 scripts/lint-spec-files.test.py` passed (19 tests).
- `python3 scripts/lint-spec-files.py --all` passed.
- Markdown link audit passed for migrated specification references; only the
  existing template placeholders and one unrelated historical application path
  remain outside the migration scope.
- `git diff --check -- docs/specs docs/decisions docs/plans` passed.

## Risks

- A source may describe more than one system; assign ownership before moving
  it, and use links rather than copied contract text for adjacent systems.
- Large legacy documents require capability or lifecycle splits to meet the
  new requirement and system-design size limits.
- Plans and decisions contain many relative links to legacy paths; reference
  repair must happen before deleting any legacy source.
- Stable requirement IDs must remain unique and must not be reused when a
  capability is split.
