# ADR-2026-11-26-sdk-npm-upload-attempts: Retain SDK npm upload attempts in Actions

**Status:** accepted
**Date:** 2026-11-26
**Area:** workflow

## Context

During [release v0.104.0](https://github.com/kdlbs/kandev/actions/runs/45816922667/attempts/1), npm accepted SDK version 0.5.0 shortly before Kandev began waiting for public visibility.
The registry recorded the version nine seconds after Kandev's five-minute deadline.
A rerun recovered the existing version.
A retry during the delay would have seen an absent public version and could have submitted the archive again.

The [SDK release evidence decision](2026-11-05-sdk-release-evidence.md) makes the signed archive and receipt immutable.
They prove the identity of a release candidate before npm receives it.

## Decision

Keep one immutable upload intent and its result in Actions artifacts for each SDK version and release run.
Retain the intent before issuing `npm publish`.
Bind both records to the signed archive and the protected release workflow.
An accepted result, an uncertain result, or an intent with no proven outcome leads to registry observation on recovery.

Use the existing release-wide concurrency group, artifact lookup, and authenticated workflow-run/job-attempt metadata to reconcile attempts across reruns and backfills.
The authenticated job history remains authoritative when retained artifacts are missing: successful intent retention with a missing record blocks another upload.
Name the publisher job with its host tag so workflow-run history identifies attempts for the signed release.
A fresh write following a proven refusal uses a new backfill run for the same signed release.
Require a complete retained history for fresh upload authorization.
The public registry and signed receipt establish publication success.

## Consequences

An interrupted runner leaves enough information for the next run to observe the version safely.
Artifact loss cannot silently turn an earlier upload into an untouched release because authenticated job outcomes provide an independent completeness check.
The workflow uses GitHub Actions storage and read permissions with the existing OIDC publisher.
The retention floor covers GitHub's 30-day rerun window.
Longer recovery or missing legacy history can require maintainer reconciliation when the version is still absent.

## Alternatives considered

- Logs and job summaries can disappear with an interrupted process before it records the publish result.
- Release assets have an immutable lifetime and are finalized before the publisher runs.
- A repository tag or deployment record would add a repository write permission and a new production resource to maintain.

## Related artifacts

- [Publish and recovery requirements](../specs/release/requirements/npm-publication-reliability.md#req-release-npm-publication-004-recoverable-sdk-upload-attempts).
- [Publication recovery design](../specs/release/system-design/npm-publication-reliability.md#recovery-and-verification).
- [GitHub artifact API](https://docs.github.com/en/rest/actions/artifacts).
- [GitHub rerun window](https://docs.github.com/en/actions/how-tos/manage-workflow-runs/re-run-workflows-and-jobs).
