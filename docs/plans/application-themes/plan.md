---
created: 2026-10-22
status: complete
requirements:
  - REQ-PLUGINS-THEMES-001
  - REQ-PLUGINS-THEMES-002
  - REQ-PLUGINS-THEMES-003
  - REQ-UI-APP-THEMES-001
  - REQ-UI-APP-THEMES-002
system_design:
  - ../../specs/plugins/system-design/theme-packages.md
  - ../../specs/ui/system-design/application-themes.md
legacy_specs: []
---

# Implementation Plan: Extensible Theme Support

## Overview

Build a generic theme ecosystem: native data-only theme packages, standalone VS
Code JSON/JSONC imports, contributor marketplace publishing, and whole-app code/
diff rendering. Monokai is an example theme, not a built-in feature to hardcode.
This package replaces the previous unimplemented Monokai-only draft; the old
work orders have been removed so they cannot be executed accidentally.

User-confirmed scope: whole application; both native and VS Code imports.
Package operations retain existing instance-admin permissions and installation-wide
inventory; choosing an installed variant remains browser-local.

The original five work orders record the implementation authorized on
2026-10-22. The [PR remediation package](../theme-pr-remediation/plan.md) records
the subsequent update-admission, VS Code default-rule, Pierre-option and
Appearance-fixture repairs and final passing verification. PR #4731 remains a
draft for remote CI and review. All implementation stayed in the primary
session; subagents were not authorized.

## Scope

### In scope

Versioned canonical theme format/schema, strict data-only package profile,
conversion/reporting, reviewed imports, lifecycle and inventory APIs/events,
generic renderer adapters, marketplace kind/filter and registry CI, contributor
how-to/sample, desktop/mobile selection/import/recovery and targeted test evidence.
Final delivery includes a seeded running instance from the implemented task build,
with a sideloaded Monokai-style theme selected and an access URL bound to the
host's Tailscale IP, left running for the user's manual testing.

### Out of scope

A built-in Monokai palette, executable theme plugins, arbitrary CSS/JS, running VS
Code extensions or VSIX, remote includes/marketplace scraping, new marketplace
infrastructure, visual theme editing, per-workspace/account preferences, personal
install libraries, fonts/icons/layout changes, automatic publication, and creating
an external production theme repository or marketplace listing.

## Ownership and technical approach

- [Plugin requirements](../../specs/plugins/requirements/theme-packages.md) own
  package/import/marketplace contracts; [UI requirements](../../specs/ui/requirements/application-themes.md)
  own independent reusable selection/presentation. Both designs are listed above.
- [ADR](../../decisions/2026-10-22-data-only-theme-packages.md) records the
  data-only trust boundary, reuse of plugin artifacts and canonical conversion.
- Task 01 establishes manifest `themes`, distribution kind `theme`, bounded JSON
  role schema, converter/report and pack/verify support. The existing managed
  binary path remains strict. Native and imported definitions share conformance
  fixtures; no language/theme-specific host branch.
- Task 02 installs definitions without runtime execution through existing records,
  locks and atomic version publication. It adds exact prepared review APIs,
  eligible inventory/content projection, lifecycle/restart handling and change
  notifications, with existing admin and Plugins admission gates.
- Task 03 adds browser theme reference resolution, complete role defaults, shared
  state/hook/API loading, sanitized cache/bootstrap and renderer adapters. It
  uses pinned Pierre registerCustomTheme and worker setRenderOptions, not a
  built-in Shiki name or pool remount. See the renderer matrix in the UI design.
- Task 04 adds Themes to the current marketplace/registry and builds the native/
  VS Code import review surfaces. It reuses source precedence and lifecycle APIs.
  Registry checks validate actual archives and require attribution/license.
- Task 05 exposes generic installed selection in Appearance and closes desktop/
  phone, lifecycle, contributor documentation and format interoperability proof.

No new release flag is proposed. Plugins are permanently available in the current host; existing authentication
and admin authorization gate inventory and management respectively. Built-in
light/dark/system remain available when inventory cannot be loaded. No SDK callback or public light/dark enum widening.

## Compatibility matrix

| Consumer | Supported input / identity | Behavior and verification | Unsupported fallback |
| --- | --- | --- | --- |
| Native archive | theme-only manifest, stable package/key, immutable digest | shared validator and no-runtime install; Task 01/02 tests | reject unsafe/mixed/incompatible before mutation |
| VS Code standalone | JSON/JSONC, supported colors/token rules, explicit base | canonical conversion + visible report; Task 01/04 tests | missing roles use base; includes/VSIX rejected |
| Monaco | portable syntax and standard semantic rules | existing models receive generated theme; Task 03/05 | complex TextMate scopes approximate, reported |
| CodeMirror | portable roles mapped to Lezer tags | reconfiguration on existing state; Task 03/05 | base for unmapped roles; report engine limitations |
| Shiki/Pierre | sanitized TextMate + generated portable rules, digest name | custom data registration; actual worker/cache test; Task 03/05 | readable plain code on load failure; stale completion fenced |
| xterm/shell/Tiptap | allowlisted app/editor/terminal roles + binary base | digest-dependent repaint/CSS, state preservation; Task 03/05 | known base; intentionally fixed-dark terminals stay independent |
| Plugin/canvas marketplace entries | existing kind or omitted kind | existing tests pass with theme entries added; Task 04 | old hosts skip new kind without losing old entries |

## ASCII UI preview

### UI-01: Appearance, shared desktop and phone composition

Entry: Settings > Preferences > Appearance. State: installed theme preview.

```text
Appearance
  Color theme [ Example Themes / Charcoal v ]
    Built-in: System, Light, Dark
    Example Themes: Charcoal, Paper
  [Import theme] [Browse themes]
  Compatibility / contrast findings, when present

       Unsaved changes  [Reset] [Save changes]
```

Keep the existing settings-content scroll owner and fixed safe-area-clear Save
surface. Built-ins remain available during inventory errors; an unavailable saved
reference is shown with its fallback and Retry rather than removed from storage.
Import is admin-only with a permission explanation for other users. Phone uses
the same focused Appearance route and responsive primitives, >=44px controls;
desktop retains 28px density. No new overlay for simple theme selection.
Map: AC-UI-APP-THEMES-001.1/.4/.5 and AC-UI-APP-THEMES-002.1/.2/.4/.5.

### UI-02: Themes marketplace, desktop and phone

Entry: existing Settings > Plugins, Themes tab, or Browse themes from Appearance.
State: list then selected listing, with admin import/install availability.

```text
Plugins: Installed | Browse | Canvases | Themes
Themes                 [Search] [Import theme]
  Example Themes   Author / source   [Details]
  Another Pack     Author / source   [Details]

Selected listing (replaces the list on phone)
  [Back]  Example Themes
  Author / source / version / license / compatibility
  Variants: Charcoal (dark), Paper (light)
  [Preview samples and compatibility report]
  [Review install]  or  Installed / Update available
```

Reuse existing marketplace list/detail/search/source patterns. Empty inventory
shows Import/Browse guidance; fetch error shows Retry without clearing installed
state. Incompatible packs show why installation is unavailable. Non-admins can
browse and select installed themes but management actions explain their restriction.
Phone list/detail is one focal view, not two panels stacked; content scrolls in
the settings region. Map: AC-PLUGINS-THEMES-003.1/.4/.5.

### UI-03: Import/install review

Entry: admin Import theme or listing Review install. State: prepared valid input.

```text
Desktop: bounded review dialog
  Theme import                           [Close]
  Identity / author / version / license
  Variants and declared light/dark bases
  Scoped code + diff + application samples
  Compatibility: mapped / approximate / ignored / fallback
  Contrast findings
                     [Cancel] [Install themes]

Phone: full-height review surface
  [Back] Theme import
  Metadata, variants, samples, report  (one scrolling body)
  [Cancel] [Install themes]            (fixed safe-area actions)
```

Unspecified imported base exposes a required Light/Dark choice before preparation
can complete. Invalid input shows field errors; expired/conflicting review offers
Prepare again; failure preserves the old install and draft. Cancel never installs
or selects. Installation success offers Go to Appearance, not auto-apply.
Use existing Dialog/Drawer primitives, dynamic viewport sizing, one internal
scroll region, keyboard focus return, >=44px phone/coarse-pointer actions and no
document overflow. Surface rationale: dense metadata/report benefits from a
focused full-height phone surface rather than a compact picker. Map:
AC-PLUGINS-THEMES-002.1/.2/.3/.4 and AC-PLUGINS-THEMES-003.5.

### UI-04: Existing task content with a contributed theme

```text
Desktop: Sidebar | Task tabs / toolbar | File editor | Changes / Review
                                        Terminal    | Chat code / chat diff
Phone:   Task header -> Current file, Changes or Chat -> Existing bottom nav
```

Theme changes colors only. Preserve existing focused phone navigation, scroll
owners, editor/terminal identity and work. Names and spacing above are illustrative;
control hierarchy, shared state, safe actions and recovery are structural.
Map: AC-UI-APP-THEMES-001.2/.3/.5 and AC-UI-APP-THEMES-002.3/.5.

## Tests and traceability

Planned test names/files are implementation targets, not execution evidence.

| Criteria | Target evidence |
| --- | --- |
| AC-PLUGINS-THEMES-001.1/.2/.3 | `themes/schema_test.go`, `manifest/themes_test.go`, `pkgtar/theme_test.go`: positive two-variant pack, unsafe/mixed/bounds/unknown fields, checksum and executable-plugin regressions |
| AC-PLUGINS-THEMES-001.4, 002.1/.2/.4 | `themes/vscode_test.go`, packer/verifier theme tests, shared fixture conformance: maps/precedence, JSONC, duplicates, missing base, scope approximation, unknown license and deterministic import identity |
| AC-PLUGINS-THEMES-002.3, 003.3/.4 | `themes_handlers_test.go`, `service_themes_test.go`: actor/expiry/digest/predecessor fencing, idempotent commit, admin gates, no-runtime lifecycle, failed upgrade rollback, restart, revision events |
| AC-PLUGINS-THEMES-003.1/.2 | `marketplace/catalog_test.go`, `marketplace_handlers_test.go`, `plugin-registry/build-index.test.mjs`, `check-releases.test.mjs`: kind/source/checksum/license verification, old catalogs and hostile archive |
| AC-PLUGINS-THEMES-002.1/.2/.3/.4, 003.5 | `theme-import-review.test.tsx`, `theme-marketplace.test.tsx`, real prepare/commit clients plus phone E2E |
| AC-UI-APP-THEMES-001.1/.4, 002.1/.2/.3 | `theme-resolver.test.ts`, `theme-bootstrap.test.ts`, store/API hook tests, provider and Appearance integration: defaults, safe cache, stale inventory/content, failed refresh versus omission and save revision handling |
| AC-UI-APP-THEMES-001.2/.3/.5, 002.3 | renderer and terminal tests: generic fixture colors, actual Pierre custom theme registration/worker update, semantic mapping, role cleanup, async failure/races, retained editor/terminal state |
| AC-UI-APP-THEMES-002.4/.5 | theme-toggle and Appearance tests plus desktop/mobile real flows, read-only permissions and built-in recovery |

## E2E tests

Use production-built frontend with disposable backend/data fixtures. No external
production repository, user instance or marketplace listing is mutated.

- `e2e/tests/plugins/theme-marketplace.spec.ts` (`chromium`) and
  `mobile-theme-marketplace.spec.ts` (`mobile-chrome`): browse Themes, prepare
  native release with two variants and exact digest, inspect report, cancel,
  install, duplicate retry, incompatible/tampered package, non-admin direct API
  rejection, update/disable/uninstall and old-install retention on failure.
  Covers all PLUGINS criteria, with bounded adversarial cases in Go tests.
- `e2e/tests/plugins/theme-import.spec.ts` and `mobile-theme-import.spec.ts`:
  upload native pack and standalone VS Code JSONC, choose missing base, inspect
  mapped/approximate/ignored/fallback and contrast findings, confirm/cancel,
  duplicate labels, reject include/unsafe input, handle expired review.
  Covers PLUGINS 001.1/.2/.4, 002.1-.4 and 003.4/.5.
- `e2e/tests/settings/application-themes.spec.ts` and
  `mobile-application-themes.spec.ts`: real Appearance select/preview/reset/
  discard/save/reload, both variants plus imported theme, OS behavior, old saved
  choice compatibility, unavailable-reference recovery, delayed response fencing.
  Exercise editor/code block/diff provider matrix, task Review/Changes/chat,
  undo/selection and terminal continuity, semantic and computed token colors.
  Covers every UI criterion and PLUGINS 003.3/.4.
- Phone scenarios assert >=44px targets, safe-area-clear fixed actions, long
  report internal scrolling, focused list/detail/review composition, keyboard
  focus return and zero document overflow. Include narrow fine-pointer coverage.
  Inspect screenshots, not just class names; these are required final evidence.

## Final running-instance handoff

Task 05 owns this user-requested delivery gate after implementation checks pass:

- Build the implemented task checkout and start a disposable instance with its own
  database, data directory, fictional repository and free ports. Seed a coherent
  task with editable source, additions/deletions in a diff, chat code and terminal
  content so the theme can be assessed across viewers. Use mock providers.
- Sideload an attributed, permissively licensed Monokai-style definition through
  the implemented import/review/commit flow. Record source, license and digest;
  select and save it through Appearance, then verify a reload at the access origin.
- Discover the host's current Tailscale address at launch and explicitly bind the
  application listener with `KANDEV_SERVER_HOST` to that IP on a free port. Use
  the built frontend served by the backend. Inspect actual listeners and verify
  health, page loading and WebSocket connectivity through the Tailscale URL.
  `scripts/dev-isolated` currently clears inherited environment with `env -i`, so
  merely prefixing that script with the bind variable does not configure its child.
- Selection remains browser-local: save the theme in the verification browser at
  the Tailscale origin and provide the exact Appearance selection for a fresh user
  browser. Do not claim that seeding server data sets another browser's preference.
- Leave the instance running beyond the agent turn. Hand off URL, sample task,
  theme name, source/build identity, data/log locations and exact stop/cleanup
  commands. Record local reachability separately from user-device confirmation.
  If Tailscale is unavailable, report that concrete blocker and keep this gate pending.

## Work orders

- [x] [01: Theme format and package validation](task-01-theme-format.md)
- [x] [02: Import and no-runtime package lifecycle](task-02-theme-lifecycle.md)
- [x] [03: Generic application and renderer pipeline](task-03-theme-rendering.md)
- [x] [04: Marketplace discovery and import review](task-04-theme-marketplace.md)
- [x] [05: Appearance, contributor guide and complete flows](task-05-theme-experience.md)

Order: 01 -> 02 -> 03 -> 04 -> 05. All sequential. Work orders list exact
verification commands and likely files. Fresh-worktree frontend commands require
the one-time frozen install.

## Verification results

Implementation and delivery completed on 2026-10-22. All five work orders are
complete. [Delivery evidence](delivery.md) records executed suites, live browser
checks, implementation choices, known compatibility limits and the running
instance's source, artifacts and lifecycle commands.

The earlier test lists are planning targets, not assertions that every proposed
filename or browser permutation was executed. Delivered coverage combines Go and
frontend regression tests, real desktop/mobile import and selection flows,
marketplace installation, and focused live renderer/continuity checks.

## Risks

- Managed-plugin assumptions span startup, sync, updates and cleanup; every path
  needs a no-runtime theme branch with regression coverage, not just installation.
- VS Code compatibility is a subset; preserve supported TextMate for Shiki/Pierre
  and report approximations for Monaco/CodeMirror rather than claiming exact fidelity.
- Plugin-managed inventory is shared and admin-only; individual selections remain
  local. This intentionally does not add personal per-user theme installation.
- Only theme references are persisted; authenticated content loads after the
  built-in base. Stale responses cannot authorize an absent theme or overwrite
  a newer effective digest.
- Pierre uses session-lived custom theme registrations without an unregister API;
  repeated version updates retain bounded definitions until the browser app restarts.
- Existing plugin custom-registry HTTP(S) trust policy remains unchanged; no theme
  file may trigger a network request. Registry digests bind the reviewed artifact.
- Newly introduced first-supporting host version must be set from the actual
  implementation release before contributor examples/registry fixtures are finalized.
