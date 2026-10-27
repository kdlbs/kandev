---
id: EXECUTOR-PROFILE-CARD-KEYBOARD-01
title: Make existing profile editors keyboard reachable
status: done
wave: 1
depends_on: []
plan: ./plan.md
requirements:
  - REQ-EXECUTORS-PROFILE-EDITOR-001
acceptance_criteria:
  - AC-EXECUTORS-PROFILE-EDITOR-001.28
system_design:
  - ../../specs/executors/system-design/profile-editor.md
---

# Make existing profile editors keyboard reachable

## Scope and acceptance

In the shared `ExecutorProfilesCard`, give each existing profile a native
primary button with visible focus and an independent sibling delete button.
Use the current canonical ID helper and router. Preserve pointer behavior,
provider availability, existing create/delete/refresh logic, and name/badge
content. No nested interactive controls, new dependencies, API, provider,
state/race, settings-wide redesign, unrelated cleanup or external plugin work.

1. Native Tab from the provider Add control reaches the matching existing
   profile's primary action. Enter and Space each open that profile's actual
   canonical editor without deletion; pointer click also opens it. Exercise
   delete separately and show it deletes the selected profile without editor
   navigation or deletion of another profile.
2. On the Pixel 5 project, tap the existing profile to open the matching editor;
   exercise the separate delete target without opening an editor. Primary and
   delete hit areas are at least 44px in both dimensions, do not overlap, and
   the document has no horizontal overflow. Inspect the rendered phone row.
3. Independent causal pre-fix browser RED and bounded post-fix GREEN pass the
   applicable commands below. Preserve actual discovery, setup, exit and join
   evidence; a setup failure, missing dependency or zero tests is not RED.

## ASCII UI preview

UI-01 (see [plan](plan.md#ascii-ui-previews)), AC .28:

```text
Desktop before:  | Profile name           [Delete] > |  Tab skips edit
Desktop after:   | [Profile name       >] [Delete] |  Tab -> edit -> delete
Phone after:     | [Profile name       >] [Delete] |  tap -> editor
```

The name/badge button fills the primary row area; delete is a sibling. These
are structural requirements, not exact spacing. Phone uses the existing page
scroll owner and direct navigation, no new drawer or state. Focus is visible;
no overlapping targets. Desktop delete is 28px and touch targets at least
44px. Preserve truncation and existing localized content.

## TDD and browser oracle

After ROOT's separate implementation release and heavy lease, independently
write `apps/web/e2e/tests/settings/executor-profile-card-keyboard.spec.ts` for
`chromium` and `apps/web/e2e/tests/settings/mobile-executor-profile-card-navigation.spec.ts`
for `mobile-chrome`. Existing fixture helpers install the packaged disposable
executor plugin and create profiles through the real Add/Create flow; always
uninstall in `finally`. Use unique selected and sibling profiles, real hub,
state, router and controls. Start keyboard traversal at the actual Add control
and press native Tab; assert focus on the primary action, then native Enter and
Space in separate visits and check the exact profile ID route and editor. Do
not focus the primary action directly as a substitute for reachability. Assert
no DELETE on edit, use actual separate deletion on another visit, check deletion
response and remaining sibling through the API, and check the URL remains hub.
A pointer case verifies the same matching editor. Phone tap follows the same
real flow and checks geometry, no overlap, overflow and deletion isolation.
Do not read, copy, or run ROOT's protected qualification source. Avoid the
existing plugin profile E2E files and do not install `user-event`.

Run discovery and RED against the unchanged production card. Record counts and
failure assertions, plus a passing pointer control. Trace actual focused elements
after native Tab from the loaded Add control; do not locate a future primary
button or test ID before driving Tab. An infrastructure, setup, selector or
discovery timeout is not RED. A behavioral focus or URL assertion timeout after
real controls load and the native Tab trace proves the blocked path, with the
pointer control passing, is valid causal RED. Implement only after the keyboard
assertion fails for its intended reason and controls run. Rerun the
bounded desktop suite GREEN and the phone suite sequentially. Use actual
component navigation tests for both card callers' shared behavior; add a
focused meaningful component test if necessary, without replacing the browser
oracle. Recheck only changed affected inputs after a correction.

## Commands and lease

Run from repository root, sequentially, after the later explicit release and
ROOT's exclusive heavy lease. Use the existing Node 24.21.0, pnpm 9.15.9 and Go 1.26.0
for every command including normal hooks; verify both versions. Create a unique
task-owned `run-quiet` directory and retain its log paths, each command's actual
session handle, JOIN, exit and counts. Never chain checks in a way that masks
an exit. Fresh worktree dependencies: `(cd apps && pnpm install --frozen-lockfile)`
once. Managed runs prepare backend, web and fixture artifacts; do not bypass
freshness.

```bash
export PATH="/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH"
export KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/kandev-child022-run-quiet.XXXXXXXX)"
node --version
pnpm --version
go version
(cd apps/web && pnpm exec playwright test --config=e2e/playwright.config.ts --project=chromium --list e2e/tests/settings/executor-profile-card-keyboard.spec.ts)
(cd apps/web && pnpm exec playwright test --config=e2e/playwright.config.ts --project=mobile-chrome --list e2e/tests/settings/mobile-executor-profile-card-navigation.spec.ts)
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/settings/executor-profile-card-keyboard.spec.ts
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/settings/mobile-executor-profile-card-navigation.spec.ts
(cd apps/web && pnpm exec vitest run --project=browser --maxWorkers=1 --no-file-parallelism components/settings/executor-profiles-card.test.tsx)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/settings/executor-profiles-card.tsx e2e/tests/settings/executor-profile-card-keyboard.spec.ts e2e/tests/settings/mobile-executor-profile-card-navigation.spec.ts)
(cd apps/web && pnpm run i18n:check)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

First desktop managed run is RED before production edit; same command is GREEN
once after. The phone run is GREEN after the fix. Discover nonzero tests for
both named projects; require nonzero Vitest collection in the `browser` project
for the existing card test, per `apps/web/scripts/vitest-reviewed-projects.json`.
Choose the documented correct project for any added unit suite and require its
nonzero collection. Scoped ESLint includes all actual added or changed TS/TSX,
including component tests and helpers. Canonical `(cd apps/web && pnpm run typecheck)`
requires an explicit resource cap chosen from current memory and the prior 4 GiB
OOM evidence: ROOT authorized `NODE_OPTIONS=--max-old-space-size=6144` for
the initial run; record the cap and result.
Do not blindly rerun OOM or resource/auth/unknown failures.

Under the delivery grant, stage exactly the four package documents and every
actual new or changed owned TS/TSX and helper input before each staged ratchet;
inspect `git diff --cached --name-only` and reconcile unrelated edits first.
For the expected paths the explicit staging command is:

```bash
git add docs/specs/executors/requirements/profile-editor.md docs/specs/executors/system-design/profile-editor.md docs/plans/executor-profile-card-keyboard-navigation/plan.md docs/plans/executor-profile-card-keyboard-navigation/task-01-profile-card-navigation.md apps/web/components/settings/executor-profiles-card.tsx apps/web/e2e/tests/settings/executor-profile-card-keyboard.spec.ts apps/web/e2e/tests/settings/mobile-executor-profile-card-navigation.spec.ts
git diff --cached --name-only
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm run e2e:sleep-ratchet)
```

Add any actually changed owned component test or helper explicitly before the
ratchets, and repeat only the affected ratchet when staged inputs change. Run
normal hooks with the same PATH, no bypass. Follow commit, push, PR and fixup
skills with the user's publication and CI restrictions.

Phone GREEN may use `--no-build` only with verified fresh desktop GREEN assets:
record and compare the HEAD, binary diff and status, hashes of every relevant
untracked input, all `apps/web/dist` files, backend binaries, fixture package,
generated fixture UI and identity immediately after desktop GREEN and before
phone. Use a deterministic sorted path/hash manifest, verify missing files as
failures, record both fingerprints and owned fixture cleanup. Never reuse RED
assets after the code change. If any fingerprint cannot be verified unchanged,
use the full managed phone command above. With verification, the phone command is:

```bash
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --no-build --project mobile-chrome tests/settings/mobile-executor-profile-card-navigation.spec.ts
```

## Files and dependencies

Likely production file: `apps/web/components/settings/executor-profiles-card.tsx`.
Likely tests: the two new focused browser files above and possibly
`apps/web/components/settings/executor-profiles-card.test.tsx`. Read-only:
`app/settings/executors/page.tsx`, `app/settings/executor/[id]/page.tsx`, the
fixture helpers, routes and control-sizing helpers. This order depends on no
other work order. The active requirement and current design are amended as the
existing owner; plan remains pending until the later implementation request.

## Mobile and documentation assessment

The existing settings hub is a direct navigation surface, like the curated
`kanban-with-preview.tsx` precedent. Shared routing and state apply on phone;
page content is the single scroll owner, and existing safe-area rules remain.
No public executor guide or screenshot needs alteration: user-facing steps and
terminology remain the same. Reassess if copy or the visible workflow changes.

## Results

ROOT released implementation and the exclusive heavy lease. Frozen install
joined handle 92814, exit 0. Desktop discovery joined 98375, exit 0, three
tests; phone discovery exit 0, one test. Pre-fix managed desktop RED joined
23579 (join e787f8), exit 1: Enter and Space each reached the actual Delete
Profile button after Tab, and the pointer/delete control passed. No protected
qualification source was used.

The first post-fix desktop run joined 20253 (join 5aa1aa), exit 1: Enter and
pointer passed but Space reached an earlier profile left by the previous test.
The test now cleans its own profiles between cases. The corrected desktop run
joined 19393 (join 1cfedd), exit 0, three passed. Fresh managed phone run joined
19008 (join 89b3b1), exit 0, one passed. Its screenshot missed the row, so the
capture was scoped to the row and the changed phone test reran with fresh build:
handle 7543, join fcbf88, exit 0, one passed. Visual inspection confirmed the
primary row and separate delete target. Browser fixture directories named in
the owned run logs were absent after runs.

Existing card Vitest in `browser`: handle 53809, join 7483d5, exit 0, two
tests. Scoped ESLint: handle 86765, join c683c2, exit 0. Canonical typecheck
with the authorized 6144 MiB cap: handle 20030, join 34d973, exit 0. i18n:check:
handle 57556, join 7c2693, exit 0. Logs are under
`/tmp/kandev-child022-profile-keyboard/`. The staged i18n ratchet joined 24233
(join 4253df), exit 0; the E2E sleep ratchet joined 66973 (join 417ee0), exit 0.
Final document checks, normal hooks and delivery follow this work order.
