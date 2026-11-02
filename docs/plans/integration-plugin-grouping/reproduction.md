# YouTrack sidebar reproduction

Date: 2026-10-25. Result: reproduced with the actual published plugin.

## Environment

- Kandev source: `595df36ed5c1cf02fd90cb972b623ad6edc53df8`.
  Production source was unchanged. The working tree contained the fix design package.
- Isolated backend built with `make -C apps/backend build` and started using
  `scripts/dev-isolated --web --no-build`. Fresh home, database, and default workspace.
- Backend port 48429, Vite port 47429. No live user instance or copied data.
- Chromium at 1440 by 1050 pixels, through the workspace-pinned Playwright CLI.
- [YouTrack v0.2.0 release](https://github.com/ahmedbally/kandev-plugin-youtrack/releases/tag/v0.2.0).
  Package SHA-256: `95878495fcbcf0886ccfd40eca944a46d14a065524760530d30513321936a5ba`.
- Package installed through Settings > Plugins > Upload. UI and backend both
  reported the plugin active at version 0.2.0.

## Procedure and observations

1. Open Settings > Plugins and upload the released tarball.
2. Open the default workspace. The sidebar displays YouTrack below Integrations.
3. Expand Integrations. Its body contains Integration settings. YouTrack remains
   a separate row below that body.
4. Reload. The same placement remains.

Browser DOM observations before and after reload:

| Check | Expected | Observed |
| --- | --- | --- |
| Original YouTrack links in the active desktop sidebar | 1 | 1 |
| YouTrack links inside `sidebar-node-integrations` | 1 | 0 |
| Nearest layout-node identity | `integrations` | `plugin:kandev-plugin-youtrack:youtrack-integrations` |

The released bundle registers `youtrack-integrations`, path `/youtrack`, with
`section: "integrations"`. The release therefore requests the correct group.
The host places its registered link outside that group.

The pre-fix capture is included with the PR screenshot evidence.

## Scope and limits

This verifies desktop placement and reload on the checked-out Kandev source.
No YouTrack server connection or credentials were needed for navigation registration.
No GitHub/GitLab connections were configured, so those provider rows are absent
from this screenshot. Phone behavior and the user's exact installed Kandev build
were not exercised. This was a browser reproduction, not a permanent automated
regression test or post-fix verification.

The browser needed existing Nix library paths to launch. The first expand attempt
used a nonexistent accessible name and timed out. Retrying with the actual
Integrations button succeeded. Neither setup issue changed application source.

The isolated browser and app processes were closed after evidence capture.

## Post-fix validation

Installed the same published YouTrack v0.2.0 tarball on a fresh isolated instance
running the corrected frontend over base `6677802efa4fac6c2ecc909cb433477c90f7fb2d`.
Desktop at 1440x1050 showed one original YouTrack link inside Integrations and
one in the entire sidebar, including after reload. Activating it opened `/youtrack`.
Phone at 393x851 used a touch-enabled browser context. Its initially collapsed
Integrations disclosure revealed YouTrack. The link measured 44 CSS pixels high.
Tapping it closed navigation and opened `/youtrack`.

The actual plugin route emitted an existing React warning about a function-valued
title. This placement repair does not change plugin route rendering or authenticate
a YouTrack server connection. Navigation placement and route activation succeeded.
Desktop and phone screenshots are published with the PR; pre-fix and post-fix
captures are kept separate from tracked source.

The review pass repeated installation on another fresh isolated instance. Desktop
and phone checks passed again. A touch-enabled 900x900 tablet context opened the
listing menu: Integrations was already expanded, contained one YouTrack link,
and opening `/youtrack` closed the menu. All three final captures were inspected
and attached to PR #4790.
