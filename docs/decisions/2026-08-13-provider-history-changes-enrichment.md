# ADR-2026-08-13-provider-history-changes-enrichment: Treat Provider History as Changes Enrichment

**Status:** accepted
**Date:** 2026-08-13
**Area:** frontend, GitHub, GitLab

## Context

The Changes panel combines local Git history with the current provider commit list. The provider read
can fail or time out while the checkout history remains valid. The current UI clears provider commits
and displays a warning in the panel body after one failed read. Several mounted consumers can also send
the same provider request at the same time.

A provider failure proves nothing about the relationship between the checkout and the published
branch. It does not prove alignment, divergence, or a rewrite. The checkout still gives the user a
useful view of local files and commits.

Successful provider evidence can prove that one history contains the other. The current merged list
does not make provider-only commits distinct, and it appends them after older local commits. This
presentation hides the useful evidence that Kandev already has.

## Decision

Kandev treats provider commit history as optional Changes enrichment.

- The Changes panel always keeps valid checkout history visible.
- The provider commit resource shares identical concurrent reads across consumers.
- The resource retries one failed read. A successful retry replaces the provisional state.
- A final provider error remains internal evidence for remote-action safety. The Changes panel does
  not show a provider-history warning or replace the checkout history.
- Remote mutations remain closed unless the available Git and provider evidence proves that the
  action is safe. Kandev does not treat missing provider evidence as alignment.
- Kandev reconciles commit lists by SHA only. It does not compare commit messages, authors,
  timestamps, file totals, or patches.
- Shared commits use the normal commit marker. Provider-only commits use a current-PR color and an
  accessible current-PR label. Checkout-only commits in confirmed divergence use a distinct
  local-checkout color and label.
- Provider-only commits appear in newest-first order with the checkout history.
- A complete provider list that contains local HEAD proves `provider_ahead` without upstream
  evidence. Pull still requires a configured upstream. Push remains unavailable while the provider
  is ahead.
- Confirmed divergence keeps the compact version-resolution control from
  [ADR-2026-08-12](2026-08-12-local-first-contribution-replacement.md). Provider failure alone never
  exposes those destructive choices.

## Consequences

- A transient provider outage does not interrupt the Changes workflow.
- Multiple Changes consumers produce one provider read for the same evidence version.
- Color gives quick provenance, and the label provides the same meaning without color.
- Provider-ahead evidence remains useful when a contribution checkout lacks an upstream.
- The frontend owns a short-lived provider-history resource with retry, deduplication, and stale-result
  guards.
- Provider errors remain available to diagnostics and action policy without becoming user-facing
  warning copy.

## Alternatives considered

### Keep the warning banner

Rejected. The warning turns a temporary enrichment failure into the main Changes state. It also asks
the user to act when no action can repair the provider request.

### Treat unavailable provider history as aligned

Rejected. Missing evidence does not prove alignment. This choice can enable an unsafe remote
mutation.

### Change the checkout to match the provider automatically

Rejected. A read failure cannot justify a reset, merge, rebase, fetch, or push. These operations can
change user work.

### Match rewritten commits by content or metadata

Rejected. Similar content does not prove commit identity or graph ancestry. Heuristic matching can
hide real divergence.
