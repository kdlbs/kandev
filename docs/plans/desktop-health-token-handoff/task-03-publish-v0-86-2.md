---
id: "03-publish-v0-86-2"
title: "Publish stable v0.86.2"
status: pending
wave: 3
depends_on: ["02-validate-desktop-release-candidate"]
plan: "plan.md"
spec: "../../specs/executors/requirements/port-collision-safety.md"
---

# Task 03: Publish stable v0.86.2

## Acceptance

- The latest Stable version is still v0.86.1, no release is active, and the validated fix commit is
  on `main`. If the Stable baseline changed, stop and recalculate instead of publishing the wrong
  version.
- Normal Stable mode publishes v0.86.2 from `main`; the release PR merge and signed tag contain the
  desktop health-token fix.
- GitHub Release/updater assets, six npm packages, versioned GHCR base/universal manifests, and the
  Homebrew formula all report 0.86.2, and a macOS v0.86.1-to-v0.86.2 update starts successfully.

## Verification

After confirming the preconditions, record the Task 02 validated SHA and dispatch the one-click
Stable patch flow from that immutable commit:

```bash
validated_sha="<task-02-validated-main-sha>"
test -n "$validated_sha"
gh workflow run release.yml --repo kdlbs/kandev --ref "$validated_sha" \
  -f channel=stable \
  -f bump=patch \
  -f dry_run=false \
  -f desktop_validation_only=false \
  -f backfill_tag=
gh run watch <release-run-id> --repo kdlbs/kandev --exit-status
release_head_sha=$(gh run view <release-run-id> --repo kdlbs/kandev --json headSha --jq '.headSha')
test "$release_head_sha" = "$validated_sha"
gh run view <release-run-id> --repo kdlbs/kandev --json headSha,jobs,url
```

Verify every channel individually:

```bash
set -euo pipefail

git fetch --tags origin && git tag -v v0.86.2
tag_sha=$(git rev-list -n 1 v0.86.2)
git merge-base --is-ancestor "$validated_sha" "$tag_sha"
gh release view v0.86.2 --repo kdlbs/kandev --json tagName,isDraft,isPrerelease,publishedAt,url,assets
for package_path in kandev \
  %40kdlbs%2Fruntime-linux-x64 %40kdlbs%2Fruntime-linux-arm64 \
  %40kdlbs%2Fruntime-darwin-x64 %40kdlbs%2Fruntime-darwin-arm64 \
  %40kdlbs%2Fruntime-win32-x64; do
  curl -fsSL "https://registry.npmjs.org/$package_path/0.86.2" | jq -e '.version == "0.86.2"'
done
gh api repos/kdlbs/homebrew-kandev/contents/Formula/kandev.rb \
  -H 'Accept: application/vnd.github.raw+json' | rg 'version "0.86.2"'
docker buildx imagetools inspect ghcr.io/kdlbs/kandev:0.86.2
docker buildx imagetools inspect ghcr.io/kdlbs/kandev:0.86.2-universal
```

Inspect the Release jobs individually. `Publish GitHub release`, `Publish npm packages`, and
`Update Homebrew tap` must all succeed. Confirm `latest.json` and both macOS updater archives are
present. Update the affected macOS installation from v0.86.1, launch it, and confirm the SPA opens
and its backend remains alive beyond the former 60-second termination point.

## Files likely touched

- Release workflow-generated version and changelog files on its release branch.
- `docs/plans/desktop-health-token-handoff/plan.md`
- `docs/plans/desktop-health-token-handoff/task-03-publish-v0-86-2.md`

## Dependencies

Task 02 and its successful validation workflow run.

## Parallelism

`sequential`. Stable publication is a single serialized workflow with immutable outputs.

## Inputs

- Task 02 validation run URL and exact `main` SHA.
- `.github/workflows/release.yml`, normal Stable mode.
- `docs/public/release-process.md`, Normal release flow and Verify every channel.
- `.agents/skills/release/SKILL.md`, Stable completion and backfill rules.

## Output contract

Report the release PR, signed tag verification, workflow run, artifact URLs/digests, each channel's
version, macOS update smoke evidence, blockers, risks, and synchronized task/plan status in the
primary conversation.

## Results

Pending.
