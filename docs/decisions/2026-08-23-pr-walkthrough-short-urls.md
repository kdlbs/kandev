# ADR-2026-08-23-pr-walkthrough-short-urls: Use 12-character SHA prefixes for PR walkthrough URLs

**Status:** accepted
**Date:** 2026-08-23
**Area:** infra, workflow

## Context

The public PR walkthrough URL currently includes the full 40-character commit
SHA. The full value is useful for workflow identity, but it makes a reviewer
link unnecessarily long. A shorter value must still identify a PR snapshot
with a low practical collision risk.

## Decision

Keep the full lowercase commit SHA in workflow inputs and validation. Use its
first 12 lowercase hexadecimal characters for the R2 object key, public URL,
and PR-description callout. The workflow and the PR-body helper derive this
prefix from the trusted 40-character event SHA.

The fixed shell loads the current website logo and favicon from the website's
stable public asset paths. This live branding is intentional. Package URLs in
the shell remain pinned, but new pages follow the current website identity.

## Consequences

- Walkthrough links are 28 characters shorter while retaining a 48-bit prefix.
- The public URL is stable for a given PR head and remains independent of the
  generated HTML bytes.
- Different commits with the same 12-character prefix can share an object
  key. The prefix is long enough for the repository's active PR snapshots and
  is safer than a conventional 7-character display prefix.
- Existing full-SHA objects are not renamed. New generations use the short
  path, and reruns update the PR callout to that path.
- Existing pages can show a later website brand asset because the shell loads
  that asset at page-load time.

## Alternatives Considered

- **Keep the full SHA:** It avoids prefix collisions, but produces unnecessarily
  long public links.
- **Use a 7-character prefix:** It is familiar in Git interfaces, but has a
  smaller collision space for a public object namespace.
- **Use a generated alias:** It can make links shorter, but it requires a
  separate alias store and lifecycle rules that are not needed for a commit
  snapshot.
