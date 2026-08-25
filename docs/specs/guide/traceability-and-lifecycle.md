# Traceability and Lifecycle

## Traceability chain

Kandev uses this traceability chain:

```text
REQ -> AC -> system design -> work order -> test
```

Requirements own product behavior. System designs own the technical path. Work
orders own implementation scope. Tests provide executable evidence.

## References

A system design lists its `REQ-*` references in frontmatter. A work order lists
the applicable `REQ-*` and `AC-*` identifiers and its system-design paths.

Use an `@covers AC-...` annotation in a test name or nearby comment when the
mapping is not clear from the test location. Do not add a second coverage ID
unless a tool requires one.

Do not copy requirement text into a system design or work order. If an isolated
execution environment needs the text, generate a snapshot and record the source
commit.

## Status values

System indexes use `draft`, `active`, or `retired`. Their `migration` field uses
`in_progress` or `complete`.

Requirement documents use:

- `draft`: The behavior is not yet accepted.
- `active`: The behavior is the current product contract.
- `deprecated`: The behavior remains documented during removal or transition.

System-design documents use:

- `draft`: The technical design is not yet accepted.
- `current`: The design describes the current intended system.
- `superseded`: Another design replaced this design.

Work orders use `pending`, `in_progress`, `blocked`, `done`, or `cancelled`.
Implementation state does not change a requirement document from `active` to
`shipped`. Delivery state belongs to plans, work orders, and coverage evidence.

## Stable identities

Requirement and acceptance-criterion IDs do not change when a file moves. Do
not reuse removed IDs. A replacement requirement references the requirement
that it supersedes.

## Migration

Migrate one system at a time. Use this sequence:

1. Create the system `README.md`, define its boundary, and set
   `migration: in_progress`.
2. Inventory the legacy specifications that the system owns.
3. Extract observable behavior into requirement documents.
4. Extract technical contracts and design into system-design documents.
5. Add stable IDs and cross-references.
6. Name the new documents as authoritative in the system index.
7. Add a short link to the system `README.md`, or archive the legacy document
   outside `docs/specs/<system>/`.
8. Set `migration: complete` and remove obsolete size exceptions.

Do not keep two editable sources of truth during migration.

Do not mark migration complete while requirement files contain generic criteria
that defer to copied legacy prose. Extract observable behavior into `AC-*` IDs.
Move technical facts into system designs, then remove the copied source detail.
