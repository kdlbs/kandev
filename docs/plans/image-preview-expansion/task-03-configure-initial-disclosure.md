---
id: "03-configure-initial-disclosure"
title: "Configure initial image disclosure"
status: done
wave: 3
depends_on:
  - "02-configure-image-expansion"
plan: "plan.md"
requirements:
  - REQ-AGENTS-IMAGE-PREVIEW-EXPANSION-001
  - REQ-AGENTS-IMAGE-PREVIEW-DEFAULTS-001
acceptance_criteria:
  - AC-AGENTS-IMAGE-PREVIEW-EXPANSION-001.1
  - AC-AGENTS-IMAGE-PREVIEW-EXPANSION-001.6
  - AC-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.1
  - AC-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.3
system_design:
  - ../../specs/agents/system-design/image-preview-expansion.md
  - ../../specs/agents/system-design/image-preview-defaults.md
---

# Task 03: Configure initial image disclosure

## Summary

Move the appearance preference to the initial inline disclosure default. Every
visible image always opens the larger dialog. This corrects the user's clarified
intent within PR #4619.

## Scope and files

Rename the setting, storage helper/key, UI state, and discovery target to describe
initial disclosure. Apply the value only as the image card's initial view state.
Remove the preference from ImagePreviewDialog activation. Update locales,
public guidance, desktop/phone scenarios, and both affected active contracts.

Primary files are `file-preview-block.tsx`, `image-preview-defaults-settings.tsx`,
`lib/settings/image-preview-defaults.ts`, and the UI slice. Do not migrate the
unshipped enlargement setting to this different meaning. Exclude new file
formats, protocol changes, backend storage, and attachment preferences.

## Acceptance

1. The default-on switch saves and resets on desktop and phone. Off starts image cards collapsed, with no read until Preview is selected.
2. A revealed image always opens the larger dialog. Retained manual disclosure choices take precedence over the saved default in both report versions.
3. Turning the preference back on restores automatic previews for cards without a retained choice. Text cards remain lazy.

## ASCII UI preview

UI-03, Settings > Preferences > Appearance. Same composition on desktop/phone.
See the [combined preview](plan.md#ascii-ui-preview). Covers AC .6.

```text
Appearance
  Show image previews by default [switch]
  Automatically show image file previews. Off starts them collapsed.
  Saved on this device.
Shared save controls: [Reset] [Save changes]

Off, fresh card: [image.png] [Preview] [Open file]
After Preview:  [image.png] [Hide preview] [Open file]
                [image button -> larger dialog]
```

Phone descriptions wrap, controls have 44px minimum touch targets, and the page
owns scrolling. The structure and behavior are required; spacing is illustrative.

## Verification

Use the renderer suite for both versions, lazy reads, enlargement, and retained
manual choices. Use the existing storage and settings-discovery suites. Extend
the shared desktop/phone browser scenario for Save, Reset, reload, collapsed
initial state, manual reveal/enlargement, and re-enabling. Run targeted Vitest,
TypeScript, focused ESLint, localization, spec checks, and public-doc validation.
Capture and attach fresh desktop/phone screenshots before updating the PR.

## Results

Completed on 2026-10-20. RED: two new renderer tests failed because images started expanded
when the preference was off. Log: `/tmp/image-defaults-correction-red.log`.


Commands from the repository root:

```sh
pnpm --dir apps/web exec vitest run lib/state/slices/ui/image-preview-defaults.test.ts components/task/chat/messages/kandev/rich-output/rich-output-renderer-file-targets.test.tsx components/task/chat/messages/kandev/rich-output/rich-output-view-state.test.tsx hooks/domains/session/use-workspace-file-preview.test.ts components/task/chat/image-preview-dialog.test.tsx lib/settings-discovery/catalog.test.ts lib/settings-discovery/search.test.ts
NODE_OPTIONS=--max-old-space-size=6144 pnpm --dir apps/web run typecheck
pnpm --dir apps/web run i18n:check
python3 /tmp/image-expansion-env.py scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --project chromium tests/chat/image-preview-preference.spec.ts tests/chat/rich-output.spec.ts
CAPTURE_PR_ASSETS=1 python3 /tmp/image-expansion-env.py scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome tests/chat/mobile-image-preview-preference.spec.ts tests/chat/mobile-rich-output.spec.ts tests/chat/mobile-message-attachment-layout.spec.ts tests/chat/mobile-image-defaults-pr-capture.spec.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node scripts/validate-public-docs.mjs
git diff --check
```

The temporary capture spec uses isolated desktop and phone contexts, waits for
finite dialog animations, and captures synthetic data through `prCapture`.
Remove it after capture; keep only the fresh ignored assets and manifest.
The environment helper is described in task 01.


Final evidence:

- 63 targeted Vitest cases passed (`/tmp/image-defaults-tests.log`). After correcting the mock state type, the six view-state cases passed again (`/tmp/image-defaults-view-state-final.log`).
- Seven desktop browser scenarios passed (`/tmp/kandev-run.e2e.uXY2syFP.log`).
- Seven phone regression scenarios and one desktop/phone capture scenario passed (`/tmp/kandev-run.e2e.Du38EoQM.log`).
- Typecheck and catalog drift passed (`/tmp/image-defaults-typecheck-final.log`). The first typecheck identified an incomplete mock AppState; the final test uses a typed subset.
- Focused ESLint and localization checks passed (`/tmp/image-defaults-eslint.log`, `/tmp/image-defaults-i18n.log`).
- Specification validation, public documentation validation (48 pages), documentation coverage, and whitespace checks passed.
- Four fresh screenshots were inspected and compressed: both viewport settings with the preference off, and both dialogs opened from manually revealed images. They match UI-03. The temporary capture spec was removed.

Initial collapse does not read file bytes. Manual preview and enlargement remain
available with the preference off. Both retained open and retained closed choices
win over the default. No new implementation limitation remains in this scope.

PR review follow-up: reserve 64px of dialog height so the 48px phone Close
button remains inside short banner previews. Add the 1600x128 phone case and
assert that the entire Close button stays inside the dialog. Give discovery
entries distinct positions matching rich-output motion, chat motion, then image
defaults. Both new regressions failed before the fix. Focused catalog, search,
and dialog tests pass (21 cases). TypeScript and focused ESLint pass.
The managed phone suite passed all seven scenarios, including banner containment
and attachment layout. The desktop rich-output suite passed all six scenarios.
Logs: `/tmp/kandev-run.e2e.SfbYQism.log` and
`/tmp/kandev-run.e2e.gQGywspz.log`. The phone RED run is
`/tmp/kandev-run.e2e.24jARUXK.log`. No product contract changed in this follow-up.

CI follow-up: frontend shard 2 failed in the attachment integration test because
it still required the retired `w-fit` dialog width. The current dialog uses a
bounded viewport width so it enlarges images on phones. Update that assertion
without changing product code. The existing phone and desktop geometry tests
remain the owner of rendered size and containment. The focused ChatMessage
suite reproduced the failure, then passed all 39 tests after the correction.
No requirements, system design, public documentation, or screenshots need an
update because this follow-up changes only a test expectation.

Post-fix verification on the PR branch passed:

```sh
pnpm --dir apps/web exec vitest run components/task/chat/messages/chat-message.test.tsx
NODE_ENV=production pnpm --dir apps/web run test --shard=2/2 --reporter=default --reporter=json --outputFile=/tmp/pr4619-shard2-results.json
```

The full shard passed 1,480 files and 13,694 tests, with four skipped tests.
Logs: `/tmp/pr4619-chatmessage-red.log`, `/tmp/pr4619-chatmessage-green.log`,
and `/tmp/pr4619-shard2.log`. CI's merge-ref report includes additional base
branch tests. The walkthrough failure was a Git checkout timeout before code
execution; its failed job was retried once.
The repaired assertion also failed when the dialog width was temporarily
changed back to `w-fit`. The production file was restored byte for byte, then
all 39 ChatMessage tests passed again. Evidence:
`/tmp/pr4619-width-mutation-receipt.json` and
`/tmp/pr4619-chatmessage-restored-green.log`. CI checked out synthetic merge
`f78a0dc46c002fd0e83bc6ac88f75007b05a6587` for the failed job.

### CI base refresh on 2026-10-21

The remaining failure was the mobile delayed changed-files reload scroll test
in E2E shard 28. CI tested merge `ad968c4fef5a0b2eeec5c8c2db3269599a0ee90a`,
with base `62a0217c93b4a57401ebbef3a2a0df4e9c5d7f2d`. Its final assertion
reported a 176px bottom gap. The dependent E2E gate failed too.

A detached reproduction matched the exact CI source tree. Its first isolated
run passed. Four CI-mode repetitions on two CPUs, with retries disabled,
reproduced the failure once with a 175px gap. Logs:
`/tmp/kandev-run.e2e.nQNK4Tot.log` and
`/tmp/kandev-run.e2e.89CfAazo.log`.

Upstream commit `00195da0c28e2811093d1176e9841d4fc790a8c6` already repairs
native scroll restoration during delayed card growth. A synthetic merge of
this PR with main `d5387f083850ffd1755854f637feaa428fca90a4` passed 145
focused unit tests, the mobile case once normally and four times on two CPUs,
the desktop counterpart, and TypeScript. Every browser run used zero retries.
The checked HEAD, index, and working tree matched the synthetic commit.
Evidence: `/tmp/pr4619-oct21-scroll-evidence.json`.

Commands used in that temporary checkout:

```sh
python3 /tmp/pr4619-two-cpu.py python3 /tmp/image-expansion-env.py scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome tests/git/mobile-turn-changed-files-scroll.spec.ts --retries=0 --repeat-each=4
python3 /tmp/image-expansion-env.py scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project chromium tests/git/turn-changed-files-scroll.spec.ts --retries=0
NODE_OPTIONS=--max-old-space-size=8192 pnpm --dir apps/web run typecheck
```

Main then advanced to `21949f6362e6830b9fb9162d2ca6ba41c5900331` through
an Azure saved-view change. Its delta does not touch the scroll or image
preview paths, and the merge remains conflict-free. A new PR revision refreshes
CI against that corrected base. The implementation and regression coverage
already live upstream. This commit records validation only, so the existing
requirements, public guidance, and image-preview captures remain applicable.
