# ADR-2026-09-27-pr-documentation-git-lookup: Resolve requirement definitions from Git objects

**Status:** accepted
**Date:** 2026-09-27
**Area:** workflow

## Context

The pull request documentation coverage evaluator receives a changed-file list,
but referenced requirements often live in unchanged files. It currently uses
GitHub code search to locate those files. One valid work order can reference
several IDs in the same document, causing several requests to an endpoint with
a separate, low rate limit. PR #3834 exhausted that limit and received an
infrastructure error although its referenced requirements were present.

The workflow runs trusted code under `pull_request_target` and can publish
statuses. It must treat pull request files as untrusted data and validate the
exact head revision, including for forks and merge-group members.

## Decision

Use Git's fixed-string search over the exact, verified PR-head tree to locate
candidate requirement documents. Fetch the PR head as Git objects into the
trusted checkout's object database. Keep the worktree and evaluator at the
trusted workflow or merge-group base revision; never check out or execute the
PR head. Verify the fetched commit ID against current PR metadata and the
merge-queue member when applicable.

Keep checkout credentials disabled. The public repository's pull-request refs
are fetched anonymously, so private-repository support is outside this decision.
An unavailable ref remains an infrastructure error; private-repository support
would need a separate credential and trust-boundary review.

Search only the owning system's requirements directory. Treat search matches
as candidate paths, then use the existing bounded exact-head document reader
and structural parser to establish definitions, acceptance criteria, and
uniqueness. Bound process time, output, candidates, and document reads. A
failed or incomplete object lookup is an infrastructure error. Remove GitHub
code search and its filename-derived fallback from this evaluator.

## Consequences

Unchanged, new, and moved requirement definitions are found from the revision
being checked. Duplicate definitions remain visible even when neither file
changed in the pull request. The workflow no longer consumes GitHub code-search
quota or depends on the default-branch search index. It still uses GitHub API
calls for PR metadata, changed files, document content, and status publication.

Git object transfer and search now need explicit identity, path, process, and
resource bounds. A moved PR head during evaluation can require a fresh snapshot;
an unavailable object must never produce a success or a false missing-definition
result.

## Alternatives Considered

- Use only the PR changed-file list: it omits unchanged requirement definitions
  and cannot detect an unchanged duplicate or validate an existing reference.
- Keep GitHub code search with longer waits or another token: retries do not
  remove its 10-request-per-minute endpoint limit, and search indexes the
  default branch rather than the exact PR head.
- Read every requirement file through the contents API: this avoids search but
  uses many API calls for systems with more than 100 requirement files.
- Require explicit requirement file paths in design frontmatter: this changes
  the authoring contract and still needs a full definition check to detect
  duplicates outside the declared path.

## Related artifacts

- [Requirements](../specs/ci/requirements/pull-request-documentation-coverage.md)
- [System design](../specs/ci/system-design/pull-request-documentation-coverage.md)
- [Implementation plan](../plans/pr-documentation-git-lookup/plan.md)
