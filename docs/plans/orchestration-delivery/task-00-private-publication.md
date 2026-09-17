---
id: "00-private-publication"
title: "Private review publication"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements: []
acceptance_criteria: []
system_design: []
---

# Task 00: Private review publication

## Summary

Publish all existing functional changes and the complete design/review package
to a private repository with public v0.94.0 ancestry. Preserve local originals
while excluding private prompts, installation data and historical secrets from
every newly reachable commit.

## In scope

1. Record the release base and original feature tree; inspect both tracked and
   untracked work. Import the feature delta into an independent clean checkout
   whose parent is `bf819a0228e742d069c528293d848c985a4d1bd1`.
2. Keep production code equivalent to the reviewed prototype. Replace identifying
   test examples with generic fixtures and replace installation-specific notes
   with reproducible guidance. Keep a local path/hash manifest for this comparison.
3. Create `Corey-Fogg/kandev-orchestration` as private; verify owner, visibility
   and permissions through the API. Disable inherited Actions before the first
   push. Set `origin` to it and disable pushes through `upstream`.
4. Publish the scope audit, pinned plugin review, canonical requirements/designs,
   all remaining work orders and candidate/runbook documentation. Provide one
   root review entry and clearly distinguish completed, partial and pending work.
5. Scan the complete new delta for credentials and identifying examples, inspect
   findings, run affected fixture tests and documentation validators, then use
   normal pre-commit and commit-msg hooks. Never bypass failed hooks.
6. Push explicit `main` at the unchanged release and the feature branch only.
   Do not mirror local backup refs or publish the original private snapshot as
   ancestry. Verify remote commit IDs, private visibility and disabled Actions.
7. Commit a sanitized publication receipt and synchronize this work order's
   status. Keep raw logs and machine-specific provenance outside the repository.

## Out of scope

New product implementation, deployment, public issue comments, public PRs,
production databases/logs, provider credentials and live conversation exports.

## Acceptance

- All existing functional changes and planning artifacts are committed to the
  verified private repository; the review link shows an exact v0.94.0 delta.
- Production-source equivalence and the deliberate synthetic fixture/doc changes
  are accounted for; new commit history contains no private prompt/history data.
- Normal hooks and affected checks pass, remote SHAs match local commits, and
  the clean checkout has a recorded review/publication receipt.

## Verification

Run from the publication checkout; authenticate using the existing Git credential
helper, never a token in an argument or URL.

```bash
git diff --check
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
python3 scripts/lint-harness-files.test.py
python3 .github/scripts/lint-harness-files.py --all
pnpm --dir apps/web exec vitest run components/task/simple/markdown-comment.test.tsx
(cd apps/backend && go test -tags fts5 -count=1 ./internal/office/service -run TestOrchestrationRosterUsesProfilesAndOwnRoutingGuidance)
git merge-base HEAD upstream/release-v0.94.0
gh repo view Corey-Fogg/kandev-orchestration --json isPrivate,owner,url
gh api repos/Corey-Fogg/kandev-orchestration/actions/permissions
git ls-remote origin refs/heads/main refs/heads/feat/workspace-orchestration
git status --porcelain
```

The source/hash comparison and redacted secret scan must include new files,
not only `git diff`'s tracked paths. Record scanner version and result counts.
An existing snapshot's broader test evidence applies only to unchanged code.

## Files likely touched

- Existing feature delta; generic examples in two existing test files.
- `WORKBENCH.md`, `docs/review/orchestration/**`.
- `docs/specs/orchestration/**`, `docs/plans/orchestration-delivery/**`.
- Existing assistant and coordinator planning packages and historical records.

## Dependencies

None. The user explicitly authorized private repository creation and publication.

## Risks

A private tip can still contain sensitive ancestor commits. Importing a sanitized
tree is insufficient if its parent is the original local feature commit. Review
the actual reachable range. Inherited scheduled release workflows must stay off.

## Parallelism

`sequential`

## Inputs

The scope audit, original v0.94.0 rebase receipt, commit/push skills and current
GitHub repository visibility documentation.

## Results

Completed 2026-09-17. Private repository and default review branch are published;
`main` is exact v0.94.0. Code commit `3536e04c` and documentation payload
`bb71b193` passed normal hooks and match GitHub's refs. Source preservation,
fixture tests, documentation checks, secret-scan triage and excluded local
history are recorded in the [publication receipt](../../review/orchestration/publication.md).
Actions remain disabled and live Kandev is unchanged. A final receipt/status
commit follows those payload commits and is verified after push.
