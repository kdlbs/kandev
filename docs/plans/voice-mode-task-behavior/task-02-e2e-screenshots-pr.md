---
id: "02-e2e-screenshots-pr"
title: "Verify responsive UI and publish PR"
status: done
wave: 2
depends_on: ["01-consolidate-settings"]
plan: "plan.md"
spec: "../../specs/ui/requirements/voice-mode-task-behavior.md"
---

# Task 02: Verify Responsive UI And Publish PR

## Acceptance

- Desktop and Pixel 5 E2E navigate through Task Behavior, prove the Voice Mode menu row is absent,
  and exercise the merged voice section without horizontal overflow.
- Fresh synthetic desktop and mobile screenshots are captured, inspected, compressed, and listed
  in a valid `.pr-assets/manifest.json` without entering the feature branch history.
- Task-defined checks pass, changes are conventionally committed and pushed, and a ready PR uses
  the repository template with both screenshots embedded under `## Screenshots`.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm e2e:run --project chromium --project mobile-chrome e2e/tests/settings/task-behavior-voice-mode.spec.ts e2e/tests/chat/mobile-voice-mode.spec.ts
cd apps/web && test -s .pr-assets/manifest.json
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
```

If the managed runner syntax cannot capture both projects in one invocation, use a single
multi-project disposable capture spec or preserve and merge the first manifest/assets before the
second run. Revalidate every manifest path immediately before publication.

## Files Likely Touched

- `apps/web/e2e/tests/settings/task-behavior-voice-mode.spec.ts` or the nearest existing desktop
  Settings spec
- `apps/web/e2e/tests/chat/mobile-voice-mode.spec.ts`
- an ignored disposable capture spec only if existing E2E cannot use `prCapture` directly
- this task file and `plan.md`

## Inputs

- Completed Task 01 UI, removed route, menu, and discovery behavior.
- `/e2e`, `/mobile-parity`, `/product-demo-seeding`, `/product-video-capture`, `/commit`, `/push`,
  and `/pr` instructions.
- The existing Task Behavior mobile exemplar and `prCapture` fixture.

## Mobile Design Contract

Enter through the Task Behavior menu row on Pixel 5, scroll the existing settings content to the
Voice Mode section, operate a representative control, and assert all required cards are reachable.
The shared settings container is the single vertical scroll owner and the document must not scroll
horizontally. Capture the native mobile page rather than cropping a desktop image.

## Risks

- The merged page is taller than the former pages; exercise the last voice card and floating save
  clearance rather than capturing only the heading.
- Capture only isolated synthetic E2E state. Validate that screenshots contain no credentials,
  tokens, personally identifiable information, or local filesystem paths.
- GitHub screenshots belong on an orphan media branch and must be SHA-pinned in the PR body, never
  committed to the feature branch.

## Results

- Desktop E2E passes for Task Behavior ownership and menu-row absence.
- Mobile E2E passes both the merged Settings page and the mobile chat mic behavior.
- Fresh synthetic desktop and Pixel 5 screenshots are captured and inspected in the ignored
  `.pr-assets` directory; the manifest contains both assets.
- Public-doc tests and `git diff --check` pass.
- Ready PR: https://github.com/kdlbs/kandev/pull/2534. Screenshot media is published at commit
  `d8e82197519c4ef6b2bb311eb7ee8e7224d166b1` on `media/pr-2534-screenshots`; no screenshot
  binaries were added to the feature branch.
