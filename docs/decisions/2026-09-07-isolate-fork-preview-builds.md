# ADR-2026-09-07-isolate-fork-preview-builds: Isolate contributor preview builds from deployment credentials

**Status:** accepted
**Date:** 2026-09-07
**Area:** infra, workflow, security

## Context

The preview workflow uses `pull_request_target` so a maintainer can approve a
fork pull request and deploy a preview. This event can access base repository
secrets. A contributor build must not run in the same process boundary as a
deployment credential.

The previous path checked out the contributor head and ran the preview command
with `SPRITES_API_TOKEN`. A build hook could read or misuse that token.

## Decision

Build fork-controlled frontend input in a tokenless job. Build the preview
command from the immutable base workflow revision in a separate trusted job.
Use a fresh tokenless package job to run only that trusted command and create a
single archive with a SHA-256 file.

Use a fresh deploy job that checks out only the immutable base workflow
revision. It must verify the archive digest and table before it passes the
archive to the trusted preview command. Only the fork deploy job receives
`SPRITES_API_TOKEN` in this flow. The preview command must fail closed when an
explicit archive is missing or is not a regular file. It may build on demand
only when the caller supplies no archive, such as the same-repository path.

## Consequences

- Contributor build commands cannot read the deployment credential from their
  job or from the tokenless package process.
- The deploy job has a smaller trusted input surface and cannot silently build
  a missing explicit archive.
- The workflow uses more jobs and short-lived artifacts.
- The deployed backend still contains contributor code and must run only in
  the isolated preview service.

## Alternatives Considered

- Keep the contributor build and deployment in one secret-bearing job. Rejected
  because build hooks could read the deployment credential.
- Remove only known credential names from the build environment. Rejected
  because future credentials or alternate process paths could bypass the list.
- Use one runner for tokenless build and trusted deployment. Rejected because
  a contributor process could remain alive across the job boundary.
