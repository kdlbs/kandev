# ADR-2026-09-26-architecture-deprecation-ledger: Track Explicit API Deprecations

**Status:** accepted
**Date:** 2026-09-26
**Area:** infra

## Context

The compatibility ledger records owners, reasons, introduction metadata, and removal conditions for intentional compatibility behavior. Explicit Go and TypeScript deprecation annotations can currently be added without an entry in that ledger. Existing compatibility annotations must remain usable while the repository adopts enforcement.

This is an internal repository check and changes no product behavior. No product requirement or system design applies.

## Decision

Add `ARCH-DEPRECATION-LEDGER` to the modular architecture linter. It recognizes handwritten production Go line-comment annotations beginning with `Deprecated:` when attached to top-level or grouped declarations, struct or interface fields (including embedded fields), interface methods, or a trailing field comment. It recognizes TypeScript JSDoc blocks containing `@deprecated` when attached to top-level declarations or class/interface/type-literal members, including nested type literals and declarations preceded by decorators. It does not infer deprecation from words in ordinary prose, comments, or strings.

The rule's exact finding identity is `(path, declaration, marker)`. `declaration` is the scanner's normalized symbol identity, and `marker` is `Deprecated:` or `@deprecated`. TypeScript string, numeric, and computed member keys use canonical bracketed names. TypeScript function and method identities include a normalized generic and parameter signature, so overloads remain distinct when source order changes. Signature whitespace and comments do not change identity, and single- and double-quoted string literal types share a canonical form. Line numbers and comment prose are diagnostic context, not identity.

If multiple annotations in one file have the same path, declaration, and marker identity, the scanner diagnoses the ambiguity and refuses ledger registration. This covers repeated identical overload signatures and multiple deprecation comments attached to one Go declaration; an occurrence suffix alone must not choose between indistinguishable annotations. See the architecture-lint guide for examples.

Compatibility-ledger entries may add `locator.declaration`. For these entries, `locator.path`, `locator.declaration`, and `locator.marker` must match one current annotated declaration. The existing ledger metadata requirements continue to apply. A matching registration satisfies the rule; each current unregistered annotation remains an exact, shrink-only rule-baseline finding. Registrations that do not match a current annotated declaration fail validation.

The first baseline contains only the current unregistered annotations. Generated files, tests, fixtures, and third-party sources are outside the rule. The scanner uses the repository's existing generated-header conventions and reports deterministic, actionable diagnostics.

Removal targets keep the existing ledger contract: a target date is calendar-expiring, while a target SemVer is a review checkpoint. This permits long-lived external compatibility to use a version checkpoint without requiring a calendar expiry.

## Consequences

New Go or TypeScript deprecation annotations must include a matching compatibility-ledger entry in the same change. The entry must identify the source declaration and retain the existing owner, reason, introduction, removal-condition, and target metadata. Existing unregistered annotations can be cleaned up incrementally; removing one requires deleting its exact baseline finding.

The architecture-lint guide documents the recognized forms and identity. Normal lint does not rewrite the ledger or baseline.

## Alternatives Considered

- **Search for compatibility words.** Rejected because ordinary domain descriptions use terms such as `legacy`, `fallback`, and `alias` without declaring an API deprecated.
- **Require every current annotation to be registered immediately.** Rejected because this would couple enforcement rollout to unrelated compatibility decisions and API removal planning.
- **Use a line number or full comment as finding identity.** Rejected because edits above a declaration or changes to explanatory prose would move the exemption without changing the deprecated API.
- **Require only a source marker in the ledger.** Rejected because a marker does not distinguish multiple declarations in the same file.
