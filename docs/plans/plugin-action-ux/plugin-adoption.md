# Official plugin adoption map

This is a delivery note, not a requirement or release certification.
The audit inspected default-branch source on 2026-09-25. It did not execute
released packages or connect real provider accounts.

Host delivery does not depend on any repository in this table. Plugin authors
can retain existing slots and adopt Action in a later release. Publication and
persistent follow-up tasks require separate user instructions.

## Audited sources

| Plugin and pinned source | Relevant contribution | Adoption work |
| --- | --- | --- |
| [Session Cost](https://github.com/kdlbs/kandev-plugin-session-cost/blob/66366f8922e869716f17a175d2ae16cb35510f2c/ui/bundle.js) | `chat-input-actions`; Button changes from icon to cost value | Replace trigger shell, preserve ref, hover/focus fetch and pinned details |
| [Provider Usage](https://github.com/kdlbs/kandev-plugin-provider-usage/blob/ff833ed25e11e162e9c49ac46684e9944a94f81c/ui/bundle.js) | Main/task topbars, right status bar, settings | Adopt topbar Action first; retain multi-provider strips until their content fits the contract |
| [Voice](https://github.com/kdlbs/kandev-plugin-voice/blob/fd7b3dfb2ec7e8017666d6f218bbfcd26e81b4c0/ui/src/composer-action.tsx) | Task chat, Quick Chat, task-create, new-session | Replace shell and CSS; preserve pointer capture, hold/toggle mode, progress, localization, and composer refs |
| [Task Manager](https://github.com/kdlbs/kandev-plugin-task-manager/blob/acf72a444e4a7c8e4f19519f87d95dcaa82654eb/ui/bundle.js) | Main topbar raw button with CPU meter | Adopt bounded icon/value trigger or retain the custom meter; keep modal and keybinding |
| [Kandy](https://github.com/kdlbs/kandev-plugin-kandy/blob/1975d8d6ebbe9b3de4e5a97d783eac4bdff54e6a/ui/bundle.js) | Task topbar raw button with 22px animated glyph | Scale artwork into the topbar glyph box; preserve preview, dialog, animation state, and data fetching |
| [GitHub Status](https://github.com/kdlbs/kandev-plugin-github-status/blob/24eee01834b7d0ee36ee7d7c2ec7b8bf14864ccd/ui/bundle.js) | Status chip plus conditional main/task topbar buttons | Adopt labelled status and icon triggers; preserve incident visibility, modal and toast behavior |
| [Template](https://github.com/kdlbs/kandev-plugin-template/blob/be2f0c51b6fca92cf752c12f4c071961276782be/ui/bundle.js) | Composer Button with copied classes | First adoption example: feature detection plus one legacy fallback |
| [Petdex](https://github.com/kdlbs/kandev-plugin-petdex/blob/ae546dc7e83c2c89322a6eefccda397921f4c439/ui/bundle.js) | Checked-in UI still matches template action and registration ID | Reconcile plugin identity before claiming mascot migration; do not infer behavior from repository description |
| [Slack](https://github.com/kdlbs/kandev-plugin-slack/blob/4c816e55536c86aa1ca2bb7224425cf836052f3c/ui/bundle.js) | Settings slot | No targeted action-slot migration needed |
| [Augpool](https://github.com/kdlbs/kandev-plugin-augpool/blob/7de23bd2f7433cd6d04823cebf393513e9e37cba/ui/bundle.js) | Route, navigation, settings slot | No targeted action-slot migration needed |
| [Bitbucket](https://github.com/kdlbs/kandev-plugin-bitbucket/blob/240dafdf9bf5c5be7ed9b857b6f3b50231de255e/ui/src/native-integrations.ts) | Native integration, review and task-link registrations | Retain existing host renderers; no raw toolbar migration needed |

## Adoption sequence

1. Update the template with the documented fallback example after host delivery.
2. Migrate simple triggers, then Session Cost and GitHub Status.
3. Migrate Provider Usage and Task Manager where the bounded content fits.
4. Migrate Voice and Kandy with their specialized interaction tests.
5. Reassess Petdex against its current identity and implementation.

This sequence does not authorize edits or releases in those repositories.
Each plugin change requires its own repository instructions and targeted tests.

## Plugin release acceptance

Before releasing an adopting plugin:

- Test the unchanged release package on the new host as a baseline.
- Test the adopting package on the new host in every registered location.
- For a fallback release, test an actual supported older host without Action.
- For a release without fallback, set the actual supporting `min_kandev_version`.
- Retain plugin registration identity and status-item ordering identity.
- Test disable/re-enable, keyboard, pointer and touch behavior, and localized labels.
- Confirm that feature detection selects one path without duplicate registration.

Do not substitute the host's old-host stub test for a real plugin release matrix.
Keep plugin-specific backend, settings, and disclosure behavior unchanged unless
that plugin's migration plan explicitly includes those changes.

## Host implementation evidence

Host checks ran against working-tree implementation based on host revision
`719a755a4525d55f1a91587f89eea2a62b01a853`. The revision names the source base;
it does not contain these uncommitted changes.

| Tested artifact | SHA-256 |
| --- | --- |
| Frozen legacy control shapes: `apps/web/lib/plugins/__fixtures__/action-compatibility/legacy-controls.tsx` | `14a454f1e2d8965444fa29dabf8f878d66f55024cae212b2374455fb479cd049` |
| Browser E2E fixture: `apps/web/e2e/fixtures/plugins/prompt-history-plugin/bundle.js` | `d85c3a0b6c2449e6ac7c3c6f8e24dfd6294a890007b29831e061e4f4410c9a4c` |
| Packaged backend fixture: `apps/backend/cmd/plugin-fixture/fixture-package/ui/bundle.js` | `d85c3a0b6c2449e6ac7c3c6f8e24dfd6294a890007b29831e061e4f4410c9a4c` |

The managed desktop browser run rebuilt the backend, Vite pseudo-locale assets,
and fixture package, then passed 7/7 tests with one Chromium worker:

```bash
(cd apps/web && pnpm e2e:run --project chromium e2e/tests/plugins/plugin-action-compatibility.spec.ts e2e/tests/plugins/composer-actions.spec.ts)
```

The compatibility browser case covered unchanged host Button and raw styled
controls beside Action, independent disclosure state, disable/re-enable, and
saved status order after reload. It observed the existing `icon-sm` host Button
at 24px and the raw metric control at 28px. The screenshot was inspected and
removed after review. Desktop composer coverage passed 6/6 cases in the same
run. The phone composer run passed 3/3 cases:

```bash
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/plugins/mobile-composer-actions.spec.ts)
```

The packaged fixture's source and backend bundle hashes match. Unit coverage
passed for the SDK/old-host contract and compatibility cases. Plugin SDK tests
and typecheck, web typecheck, i18n checks, focused ESLint, public-doc validation,
document catalog validation, specification tests/lint, and `go test
./cmd/plugin-fixture` passed. Exact Task 04 and Task 05 commands and results are
recorded in their work orders and the implementation plan.

The old-host branch is verified with a host stub, not a released older Kandev
build. The legacy controls are attributed structural fixtures derived from the
pinned source audit, not published package bundles. No official plugin release,
provider account, or live integration behavior was tested; this host work does
not certify release compatibility.
