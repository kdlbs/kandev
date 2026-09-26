---
status: current
system: architecture-lint
requirements:
  - REQ-ARCHITECTURE-LINT-DEPRECATION-001
---

# Explicit deprecation tracking system design

## Purpose and boundaries

The architecture-lint system owns this repository rule and its compatibility
ledger contract. The CI system owns workflow execution. Product systems own
customer-facing API behavior and compatibility promises.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-ARCHITECTURE-LINT-DEPRECATION-001` | Components, declaration identity, validation flow, failure behavior |

## Components and responsibilities

- `scripts/architecture_lint/rules/deprecation_ledger.py` scans supported Go
  and TypeScript annotations and creates exact findings.
- `scripts/architecture_lint/compatibility.py` validates declaration locators
  alongside the existing compatibility metadata and expiry rules.
- `config/architecture-lint/compatibility-ledger.json` records intentional
  compatibility owners and removal conditions.
- `config/architecture-lint/deprecation_ledger.json` retains only current
  unregistered annotations during rollout.
- `scripts/lint-architecture.py` runs the modular rules. The pre-commit hook
  and CI architecture check use the same repository-owned entry point.

## Data and contracts

The finding identity is `(path, declaration, marker)`. Go declaration names
include their container where required, such as `field:Payload.Old`. TypeScript
member names include their owning type or nested member path. TypeScript
function and method declarations also include normalized generic and parameter
tokens so overloads do not depend on source order. Comments and line numbers
are diagnostic context, not identity.

A declaration-level ledger locator supplies the repository-relative path,
exact declaration identity, and canonical marker. Existing ledger validation
continues to require a stable ID, owner, reason, introduction metadata,
removal condition, and one date or SemVer target. A date target expires on its
date; a SemVer target is a review checkpoint. Ordinary compatibility entries
without `locator.declaration` keep their existing marker semantics.

## Control flow

The linter scans tracked production sources, excludes generated and non-product
source classes, and creates findings only for attached canonical annotations.
It checks each declaration locator against the current findings, applies
existing compatibility metadata validation, then compares remaining
unregistered findings with the exact rule baseline.

## Failure behavior

An unregistered declaration fails with its stable identity and remediation.
A declaration locator that no longer matches fails validation. Expired or
malformed compatibility metadata keeps the existing ledger failure behavior.
Repeated indistinguishable annotations fail as ambiguous and cannot be
registered. Removed baseline findings must be deleted from the baseline in the
same change.

## Persistence

The compatibility ledger and the rule baseline are tracked JSON files. No
database or runtime state is involved.

## Security

The scanner reads source as text and does not execute it. It ignores ordinary
comments and strings, and excludes generated, test, fixture, and third-party
sources.

## Observability

Source diagnostics include the source path and annotation line. Ledger
validation points to the ledger entry and path. The local linter and CI emit
the same deterministic message for the same tracked revision.

## Related decisions

- [Architecture deprecation ledger decision](../../../decisions/2026-09-26-architecture-deprecation-ledger.md)
