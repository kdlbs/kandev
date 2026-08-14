# ADR-2026-08-10-no-em-dash-public-copy: Keep Em Dashes Out of Public Copy

**Status:** accepted
**Date:** 2026-08-10
**Area:** frontend, infra

## Context

Em dashes appear in UI copy, translation catalogs, and public documentation. They
make the product text look inconsistent and can resemble generated prose. The
repository already checks UI copy, but the public documentation validator does not
check the same rule.

## Decision

Kandev user-facing copy must not contain the Unicode em dash (U+2014). This rule
covers web source strings, locale values, backend-rendered copy, and Markdown under
`docs/public/**`. The existing no-em-dash scanner remains the shared source check;
the public-doc validator also runs its Markdown scan so documentation-only changes
fail in the published-docs workflow.

Comments, generated changelog history, and non-public internal prose are outside
this rule. Violations report the file and line, or the catalog key when the value is
in a locale catalog.

## Consequences

New UI and public-doc copy gets a single, deterministic CI check. The initial
cleanup removes existing em dashes from published pages. Authors must use a period,
colon, comma, semicolon, parentheses, or a normal hyphen when the meaning requires
one.

The scanner runs in both the web i18n check and the public-doc validation workflow,
so either kind of change remains covered when the other area is not touched.

## Alternatives Considered

- **Use an ESLint-only rule:** rejected because locale JSON, Markdown, and backend
  rendered copy are not all ESLint inputs.
- **Check only changed lines:** rejected because existing public pages would remain
  non-compliant and the rule would depend on diff-base behavior.
- **Allow em dashes in public docs:** rejected because the requested style boundary
  applies to both UI text and published documentation.
