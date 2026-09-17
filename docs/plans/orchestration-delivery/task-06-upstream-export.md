---
id: "06-upstream-export"
title: "Focused upstream contribution export"
status: pending
wave: 8
depends_on: ["05-dogfood-evidence"]
plan: "plan.md"
requirements: []
acceptance_criteria: []
system_design:
  - ../../specs/orchestration/system-design/coordinator-view.md
---

# Task 06: Focused upstream contribution export

## Summary

Prepare reviewable public contributions from the private workbench without
publishing private operational history or forcing the unfinished assistant into
the first coordinator PR. Retained public ancestry makes normal patch export
possible even though the repository is outside the public fork network.

## In scope

1. Re-read issue #3752 and current maintainer guidance. Confirm contribution scope
   and actual target branch before PR creation. Keep the private integration
   baseline at v0.94.0 unless the user separately authorizes a newer base.
2. Draft a dependency map for core run/auth/privacy seams, Orchestration storage/
   roles/chat/callbacks, central view and optional Automation. Assistant product
   work is a separately discussed series. Retain mandatory migrations/ownership
   invariants even if the associated assistant UX is excluded.
3. Create a fresh export checkout based on the agreed public branch. For focused
   later commits use `git cherry-pick`; for the initial snapshot construct scoped
   patches from the exact base and apply them with `git apply --3way`. Never push
   the private integration branch, private-only docs or backup refs to the fork.
4. Make each extracted slice buildable. Remove dependencies through explicit
   adapters/ports with regression tests rather than deleting guards. Run normal
   hooks and all affected persistence/auth/browser checks in the export checkout.
   Detect newly required upstream APIs rather than importing the plugin's newer
   host dependency silently.
5. Reconcile scope audit/README/docs with the actual exported feature. Preserve
   MIT notices for any subsequently copied plugin source; none is copied now.
   Explicitly record Automation export limitations and deferred assistant scope.
6. Capture fresh feature screenshots and short silent video from an isolated
   fictional workspace at the export commit, including desktop/mobile central
   tasks plus chat. Review every frame for prompts, tokens, accounts, paths and
   background UI. Old chat-only media does not prove the new task view.
7. Scan the entire outgoing commit range and compare it to the intended file
   inventory. Prepare title/body around the concrete behavior, limitations and
   validation. Use the repository PR skill and its asset checks before creating
   the PR when instructed; do not post public messages during preparation alone.

## Out of scope

Publishing the private repo, automatic release/deployment, bypassing maintainer
discussion, copying the plugin scheduler wholesale and an all-features giant PR.

## Acceptance

- The export branch contains only agreed source/docs, retains required guards and
  passes the scoped checks against the actual proposed public base.
- Reviewers receive fresh synthetic task-view/chat evidence and an accurate
  scope/validation description; no private ancestor or operational artifact leaks.
- The local review record links the private source commits to each public patch
  and, once instructed to submit, the resulting PR and verified head SHA.

## Verification

Run in the export checkout, where `<agreed-base>` is an actual fetched ref:

```bash
git log --oneline '<agreed-base>..HEAD'
git diff --stat '<agreed-base>...HEAD'
git diff --check '<agreed-base>...HEAD'
git status --porcelain
python3 scripts/lint-spec-files.py --all
```

Execute delivery 02's affected checks against the exported code, not only the
private integration tree. Scan new commit history before the first public push.
Use explicit remote/ref pushes after verifying the authorized destination.

## Files likely touched

Export-only branch commits and their contribution docs; private
`docs/review/orchestration/upstream-map.md` for source-to-patch traceability.

## Dependencies

Delivery 05 for daily-use evidence, coordinator-view 03 for media, and maintainer
scope/target agreement. Extraction design may start earlier; do not block useful
review indefinitely on unrelated pending assistant work.

## Risks

The private snapshot mixes necessary core seams and optional assistant work.
File-only splitting can break ownership or required stores. Validate dependency
closure and behavior, not only a small diff statistic.

## Parallelism

`sequential`

## Inputs

Issue #3752, scope audit, plugin review, source-preservation manifest and PR skill.

## Results

Pending. No public code push or PR is part of this private publication.
