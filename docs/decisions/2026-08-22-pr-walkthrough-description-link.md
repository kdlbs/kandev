# ADR-2026-08-22-pr-walkthrough-description-link: Own a top-level PR walkthrough callout

**Status:** accepted
**Date:** 2026-08-22
**Area:** workflow, infra, security, GitHub

## Context

The hosted walkthrough must be easy to discover from the pull request after
publication and after merge. Pull request bodies have no universal heading
structure, and contributors or other automations may edit them throughout the
pull request lifecycle. Updating an arbitrary section would either make the
link hard to find or risk replacing content the walkthrough workflow does not
own.

## Decision

After R2 publication and public validation succeed, a separate workflow job
places a marker-delimited walkthrough callout at the beginning of the pull
request body. The callout uses a GitHub alert and a prominent link to the
stable SHA-keyed walkthrough URL. A label rerun may replace the bytes at that
URL for the same head SHA.

The updater owns only the content between its start and end markers. A rerun
replaces that block in place, so the current generated walkthrough is linked
without duplicating content. If no owned block exists, the updater prepends
one and preserves the existing body below it. Malformed, unbalanced, or
duplicate marker blocks fail closed instead of rewriting the description.

The GitHub write runs only after the publish job succeeds. It receives the
minimum `pull-requests: write` permission, checks out only the immutable base
commit for its trusted helper, validates the event PR number and head SHA
against the URL, and receives no model or R2 credentials.

## Consequences

- The walkthrough is the first content reviewers see, independent of the
  contributor's chosen headings.
- Label-triggered regeneration updates one stable callout rather than adding
  comments or duplicate body sections.
- Contributor-authored and third-party body content remains outside the
  walkthrough workflow's ownership boundary.
- A successful upload can still be left without a body link if the GitHub
  update fails; the workflow reports that failure and the hosted object remains
  available from the publication summary.

## Alternatives Considered

- **Insert under an existing heading:** preserves the opening paragraph, but
  depends on a heading that many pull requests do not have and is less visible.
- **Append at the bottom:** avoids changing the opening, but existing generated
  sections and preview blocks can make the walkthrough difficult to find.
- **Post a pull request comment:** leaves the description untouched, but the
  durable entry point is easily buried by review activity.
- **Rewrite the full body from a template:** provides total layout control, but
  would overwrite content owned by contributors and other automations.
