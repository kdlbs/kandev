# ADR-2026-08-22-pr-walkthrough-r2-hosting: Host PR walkthrough HTML in Cloudflare R2

**Status:** accepted
**Date:** 2026-08-22
**Area:** infra, workflow, security

## Context

The PR walkthrough workflow generates a single deployable HTML file that should
remain available after a pull request merges. The file is generated once when
the PR is opened, with an explicit label-triggered rerun, so the hosting layer
needs durable object storage rather than a short-lived CI artifact. The initial
retention target is several months, with future support for generating a new
object on every push if the model cost is acceptable.

## Decision

Publish walkthrough HTML files to a dedicated Cloudflare R2 bucket named
`kandev-pr-walkthroughs`, exposed through the custom domain
`walkthrough.kandev.ai`. Store objects under
`pr/<pull-request-number>/<short-head-sha>.html`, where `short-head-sha` is the
first 12 lowercase hexadecimal characters of the exact head SHA. Publish only
the HTML, not the JSON generation artifact. The URL naming rule is recorded in
[ADR-2026-08-23-pr-walkthrough-short-urls](2026-08-23-pr-walkthrough-short-urls.md).

The GitHub Actions publication job uses a bucket-scoped R2 S3-compatible
Object Read & Write credential. The generation job receives no R2 credential;
it hands its trusted-renderer output to the publication job through a GitHub
Actions artifact. The publication job validates both the stored object and
its public HTTPS URL, then writes the URL to the workflow summary without
receiving GitHub write permission. A separate post-publication job may consume
the validated URL under the ownership rules in the PR description-link ADR.

The bucket uses a lifecycle rule for the `pr/` prefix. The initial rule
expires objects 180 days after upload so a walkthrough normally survives the
period before its PR merges. Exact retention measured from merge time is
deferred to a later merge-promotion design.

## Consequences

- Walkthrough URLs survive PR merge and GitHub artifact retention.
- R2 lifecycle cleanup is automatic and applies to all current and future
  SHA-keyed walkthrough objects.
- Label reruns can replace the object bytes for the same PR head without
  changing its stable, commit-derived URL. The URL identifies the snapshot
  scope, but it is not a content-addressed immutable object.
- Per-push generation can be enabled later by adding `synchronize`; each head
  SHA will receive its own object and lifecycle cleanup will remove old ones.
- The public bucket must contain only sanitized walkthrough HTML and must be
  served from a dedicated subdomain rather than the main app origin. A page may
  load externally hosted scripts, styles, and fonts through exact-version URLs
  controlled by the trusted shell. These third-party hosts are an explicit
  runtime trust dependency. R2 still stores one HTML object per walkthrough
  and no separately deployed asset bundle.
- GitHub Actions needs two R2 secrets in addition to non-secret bucket,
  endpoint, and base-URL variables.

## Alternatives Considered

- **Cloudflare Pages:** It can serve the HTML and is already used elsewhere in
  the repository, but its deployment history requires explicit cleanup rather
  than a simple per-object lifecycle rule.
- **Sprites preview environment:** It can serve static files, but it couples
  retention and access to the preview environment instead of providing a
  dedicated artifact store.
- **GitHub Actions artifacts:** They are useful as the generation-to-upload
  handoff, but they are not the durable public URL and retention mechanism.
