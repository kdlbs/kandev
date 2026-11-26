# Theme support delivery

Initial implementation on 2026-10-22 was committed in draft PR #4731 at
`3df5376052aab41851df584802dcafb68d7db46e`, based on
`8ba5b6909139ec2e9e14b0e2eb2f96a4e342e88b`. The original demonstration and
validation notes below describe that earlier frozen build.

## PR remediation on 2026-10-23

The [remediation package](../theme-pr-remediation/plan.md) supersedes the original
validation claims for the four reported blockers. Theme updates now bind kind,
ID, version and digest before installation; manual updates use prepare/commit.
Unscoped VS Code defaults import with explicit-color precedence. Pierre palette
changes preserve all other diff render options. Representative auth/theme
fixtures restore all 18 existing Appearance regression cases.

Final local evidence: 89 frontend tests across 15 files; full plugin/package CLI
suites; focused theme update/review/lifecycle race tests; boot/gateway theme
checks; 28 registry tests; web typecheck with an 8 GiB Node heap; changed-file
ESLint/Go lint; documentation checks; and 3 desktop plus 2 phone E2E scenarios
passed. The browser flows cover catalog installation/update and VS Code import,
selection, save/reload and unavailable-theme recovery. No new demo instance was
published for this remediation. The PR remains draft pending remote checks and
review; local results alone do not assert merge readiness.

## Try it

- Instance: <http://100.122.240.70:36693>
- Seeded task: <http://100.122.240.70:36693/t/1cbd85bb-d2be-4156-ab65-47cd75516336>
- In a fresh browser, open Settings > Preferences > Appearance, select
  **Monocai Color Theme / Monocai**, then **Save changes**. The package is
  already installed; selection is local to each browser. The earlier Monokai
  study sample remains available separately.
- Open `palette.ts`, Changes, chat code and the integrated terminal to compare
  the palette. The repository contains real sample additions/deletions.

The listener binds specifically to `100.122.240.70:36693`. Health, page loading,
WebSocket frames, theme inventory and saved-theme restoration were verified
through that address from this host. Reachability from another Tailscale device
has not been independently confirmed.

## What shipped in this checkout

Data-only theme packages share the plugin package, lifecycle and marketplace
infrastructure without starting plugin code. Strict schemas, checksums, bounded
archives and immutable content digests protect the import boundary. Prepare and
commit preserve reviewed bytes, actor ownership and the previous installation.
Themes survive restart and disappear from eligible inventory when disabled or
corrupt. Registry tooling supports contributor theme releases and verifies the
artifact kind, license and digest. No external release or listing was published.

Standalone VS Code JSON/JSONC imports produce a visible compatibility report.
The generic rendering pipeline covers app chrome, Monaco, CodeMirror,
Shiki/Pierre and theme-following xterm terminals. Appearance supports preview,
save, recovery and built-in fallback. Import uses a desktop dialog and a phone
review surface with contained scrolling and touch-sized actions.

Public docs updated: `docs/public/appearance.md`, `themes-authoring.md`, and the
plugin overview, authoring and manifest guides. The registry README describes
contributor entries. Requirements and system designs are active/current; all
five work orders are done.

## Executed validation

- Go plugin, theme, package, marketplace and CLI regression suites:
  `go test -trimpath ./internal/plugins/... ./cmd/plugin-pack ./cmd/plugin-package-verify`.
- Theme lifecycle race tests and boot/gateway theme tests passed. The final
  archive-expansion regression failed before its fix and passed afterward.
- Relevant frontend tests: **64 files, 343 tests** passed. Final targeted edits:
  **28 files, 123 tests** passed (overlaps the broader run).
- Registry: **46 tests** passed. Public-doc validator tests: **63 tests** passed.
- Web typecheck, changed-file ESLint, changed-file Go lint, localization checks
  and ratchet, public-doc validation, spec/catalog validation and whitespace checks.
- Production-build desktop E2E: `application-themes.spec.ts` and
  `theme-marketplace.spec.ts`, **2 tests** passed. Mobile:
  `mobile-application-themes.spec.ts`, **1 test** passed. Flows cover import,
  review, save/reload, disable fallback, recovery and marketplace installation;
  phone assertions check touch targets, viewport containment and overflow.
- Live browser: actual Monaco background/syntax and Pierre worker keyword color,
  xterm ANSI palette, retained unsaved editor content/undo and terminal buffer
  across theme disable/re-enable. Saved theme and both inventory variants restored
  after restarting the final binary. No browser page errors were observed in the
  final smoke, which also received WebSocket frames.

The broader scenario lists in the original plan are design targets; every
renderer, viewport and failure permutation was not exercised in a browser.
Engine-specific token mapping, schema validation, stale responses and lifecycle
failure paths also have focused unit coverage.

## Compatibility choices

VS Code imports support a documented subset. Complex TextMate scopes can be
approximate in Monaco/CodeMirror; missing roles inherit the declared base.
Includes, executable theme payloads and VSIX are rejected. Contrast findings
are advisory and do not rewrite author colors. External IDEs, authored images
and intentionally fixed-dark terminals retain their own appearance.

Only the selected reference is persisted, not the optional shell-color cache.
The built-in base paints while authenticated inventory/content loads. Shiki and
Pierre custom registrations live for the browser session because their public
APIs do not provide unregister operations.

## Instance ownership and artifacts

Disposable root: `/tmp/kandev-theme-demo-f3ni9kmb`.

This root owns the database, home/data directory, fictional Git repository,
mock providers, frozen binaries/assets, logs and verification browser profile.
The developer's normal instance and data were not modified.

The sideloaded package is an original MIT-licensed Monokai-style study fixture:
`apps/backend/internal/plugins/themes/testdata/sample`. Its stable package ID is
`theme-study`, version `1.0.0`; the archive is `theme-study-1.0.0.tar.gz` in the
instance root, SHA-256
`38efc7f2871a5c74822ff0766007de868f4bbc3b7ba98c755c2c1b02967b0ebe`.
It was packed with the real CLI, verified and installed using browser upload,
review and confirmation. It is test support, not a production plugin repository.

`instance.json` records the current process, environment, task/session IDs and
binary/archive hashes. `verification.json` records the final browser smoke.
`editor-final.png`, `diff.png` and `appearance.png` capture the inspected UI.
`validation/` holds command logs; runtime logs are `backend.log` and
`home/logs/backend-logs.log`. The instance remains running for testing.

Stop from this checkout:

```sh
scripts/kandev-kill --pidfile /tmp/kandev-theme-demo-f3ni9kmb/backend.pid --yes
```

Restart after stopping:

```sh
python3 /tmp/kandev-theme-demo-f3ni9kmb/start.py
```

After stopping, remove `/tmp/kandev-theme-demo-f3ni9kmb` to discard the sample
instance and its data. This is temporary storage and will not survive host cleanup.

## Monocai correction requested after delivery

The requested reference is **Monocai Color Theme by Berzan Yildiz**, JetBrains
Marketplace plugin 12163, not the earlier Monokai-inspired sample. The exact
published 2.0.2c JAR was retrieved from
<https://plugins.jetbrains.com/files/12163/71934/monocai.jar> and read as data.
Its `Monocai.theme.json` and `themes/Monocai.xml` supply the mapped palette.
The marketplace declares MIT licensing; attribution and license are retained.

The port is a separate native package, `monocai@2.0.2`, variant `dark`, installed
through the existing browser import/review flow. It adds no IntelliJ importer
or host changes. The archive `monocai-2.0.2.tar.gz` is in the disposable root,
SHA-256 `312570c4912f1eda8574a409d6494ab34122d32761319d41d32640a53e5bc378`.
`monocai-package/` contains the native source; `monocai-upstream/` contains the
original artifact, extracted definitions, provenance, per-role mapping and
reproducible mapping script. No external repository or listing was published.

Actual source colors include UI background `#2d2a2f`, editor background
`#2d2a2e`, keyword `#ff6188`, string `#ffd866` and number `#ab9df2`.
Mapped colors are preserved, including keyword bold/italic style. Rendering is
not pixel-identical to IntelliJ: grammars, semantic classification, fonts,
parent-scheme inheritance and host-specific surfaces differ. Generic comments
use the explicit documentation-comment style; unspecified types use scheme text.

Package verification, browser installation, selection/save/reload, actual Monaco
background and keyword color, and actual Pierre worker keyword color passed.
Phone Appearance was rendered and inspected with no document overflow.
`monocai-verification.json`, `monocai-editor.png`, `monocai-diff.png` and
`monocai-phone.png` record the checks. Installed inventory is shared across
workspaces; the verification browser now saves `theme:monocai/dark`.

## Plugin-tab layout correction

The added Themes tab had an unconditional 44px minimum height inside a 32px
shared bar. Flex wrapping used the taller item to position the row, pushing the
other tabs below the bar. The consumer now overrides the orientation-specific
bar height to auto and gives all tabs equal heights: 28px on desktop, 44px on
narrow or coarse-pointer screens. Inactive tabs use the theme foreground at 80%
instead of the low-contrast muted text role. The selected tab uses the theme
background for a distinct inset surface. The palette itself is unchanged.

Regression tests failed before the correction. Two desktop/narrow-fine-pointer
and two phone/wide-touch E2Es passed after the production build. ESLint,
formatting and whitespace checks passed. The running instance received the new
frontend assets; live Monocai checks confirmed containment and inactive-tab
contrast of 7.28:1. Desktop and phone captures were inspected. Evidence is in
`tabs-verification.json`, `tabs-fixed-desktop.png`, `tabs-fixed-phone.png` and
`validation/kandev-tabs-*.log` in the disposable root.
