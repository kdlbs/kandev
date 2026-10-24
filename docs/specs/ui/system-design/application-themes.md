---
status: current
system: ui
requirements:
  - REQ-UI-APP-THEMES-001
  - REQ-UI-APP-THEMES-002
---

# Extensible Application Theme Rendering

## Boundary and existing source

UI owns selection and rendering; Plugins supplies validated installed assets
through the [theme package design](../../plugins/system-design/theme-packages.md).
This replaces the earlier unimplemented built-in-Monokai draft. No theme name
gets a new branch or application stylesheet. Follow
[the data-only contribution decision](../../../decisions/2026-10-22-data-only-theme-packages.md).

`components/theme/app-theme.tsx` owns raw browser `theme`, preview/commit/restore,
and binary `resolvedTheme`; `lib/settings/types.ts` repeats its selection type.
`general-settings.tsx` owns the Appearance contributor and submitted revisions.
The active SPA tokens are in `apps/web/app/globals.css`, not the duplicate palette
in `apps/packages/theme/src/globals.css`. StateProvider exists above AppShell;
ThemeProvider can consume state without reversing the state-to-component boundary.

## Selection, inventory, and effective theme

Create dependency-neutral theme types/resolver in `lib/theme/`, keeping imports
out of components and state. Preserve raw stored `system`, `light`, and `dark`.
Contributed selection serializes as `theme:<package-id>/<variant-key>`, checked
against the grammar and authoritative inventory, never a label or arbitrary URL.
The stable reference follows an eligible installed update; content digest denotes
an immutable revision and is a separate property. Re-export the shared selection
type from `lib/settings/types.ts` instead of introducing competing unions.

ThemeProvider exposes `resolvedTheme: light|dark` unchanged plus `effectiveTheme`
with reference, base, package version, content digest, resolved roles and status
(loading/ready/unavailable/error). Async inventory/content state lives in the
Plugins store slice and domain hook/API layer. Add `useThemeInventory` and
`themes-api.ts`; hydrate descriptors from authenticated boot state when available,
then subscribe/refetch through the inventory protocol. Fetch only active/preview
content, not every installed theme. A request captures auth/store identity,
inventory boot/revision, reference and digest. Ignore stale results after any
owner/input changes. Do not import UI modules from state.

Unknown built-in strings resolve to System. A well-formed unavailable theme
reference stays saved but effectively uses the last known declared base, or
System if unknown. Appearance shows the unavailable reference and a built-in
recovery choice. A failed refresh retains the last acknowledged inventory;
an authoritative successful omission revokes eligibility. Re-enable/reinstall
can restore the saved reference automatically when eligible again. Never clear
it merely because of transient failure or a phone visit.

## Canonical tokens and CSS application

Resolve a complete theme fresh from a declared built-in base plus normalized
role overrides. Apply one binary root class, the CSS color-scheme, and a
host-generated identity attribute. Use host-owned fixed CSS variable names and
validated literals through style.setProperty; theme input cannot supply property
names, selectors, CSS text, URLs, geometry or markup. Remove every prior custom
property before applying the complete new result, and remove the custom block
when returning to built-ins. Keep legacy base styles unchanged.

The semantic role inventory matches current app tokens, including paired text,
status, sidebar, chart and Git colors. Convert canonical hex Git colors to the
existing space-separated RGB representation for --git-addition/--git-deletion.
Use foreground/background pairs in menus and primary actions under
[interactive accent semantics](../../../decisions/2026-07-29-interactive-accent-surface-semantics.md).
Provide code, diff and terminal-specific CSS variables where those surfaces
need colors distinct from app background. Tiptap and Dockview inherit app tokens;
replace only hardcoded theme-dependent local colors blocking that contract.
Browser/PWA titlebar metadata subscribes to effective-theme digest, not only
binary mode. Host-styled plugin UI inherits CSS; host.theme stays binary.

No new Monokai constant, theme stylesheet per package, eval, plugin JS callback
or runtime theme registration is required. A valid two-variant test pack must
work without a matching name anywhere in application code.

## Runtime renderer adapters

| Engine/surface | Generic adapter and compatibility |
| --- | --- |
| Monaco editor/diff/code block | Generate standalone theme data from canonical editor + portable syntax roles and supported semantic rules. Reuse a host-owned custom theme slot, updating its definition when digest changes. Keep model/undo state and built-in light/dark definitions. Monaco's Monarch tokenization is not full TextMate; complex imported scopes use portable approximations and are reported. |
| CodeMirror editor/code block | Build an extension from canonical editor colors and Lezer syntax tags. Reconfigure themes on existing state. Use the existing installed APIs/dependencies; fix the editable surface's hardcoded vscodeDark while preserving other extensions. |
| Shiki chat code | loadTheme with sanitized TextMate rules under a host-generated digest name, or generate rules from portable roles when no TextMate data exists. Cache by source/language/digest. Guard async result publication and show readable plain code on load failure. |
| Pierre file/chat/Review/Changes diffs | Register sanitized custom theme via registerCustomTheme; use useWorkerPool().setRenderOptions with its host-owned digest name after resolution, preserving word diff options and lazy grammar loading. Pin one shared worker pool. Per-view themeType uses binary base. The public 1.1.22 worker method distributes resolved themes, clears AST caches and rerenders subscribed viewers. Serialize/coalesce requested switches so a late request cannot leave worker colors on an older theme. |
| Theme-following xterm | Resolve background/foreground/cursor/selection and optional ANSI overrides from canonical roles. Omitted ANSI values retain existing light/dark palettes. Repaint on effective digest; preserve process, socket and buffer. Intentional fixed-dark terminals remain independent. |

Portable syntax roles, supported Monaco semantic types and CodeMirror tag maps
are versioned host mappings. TextMate engines consume preserved supported rules;
no claim of universal token-level VS Code fidelity is made. Do not replace the
language engine or add Shiki-to-Monaco solely to hide unsupported-scope reports.
Inspect all old boolean highlightCode callers and theme-dependent memo keys.

Register/compile only selected or previewed definitions, reuse by digest, and
release application-owned caches on owner change. Public highlighter APIs own
engine theme cleanup. Monaco uses a bounded slot rather than one permanent name
per update. Pierre's installed API has no theme-unregister function; retain
session-lived registrations by digest, account for their bounded per-definition
size and test repeated updates. Do not mutate library-private registries or
recreate editor models to reclaim theme entries. A later long-lived registry
budget can be added only with evidence; it is a known engine limitation.

## Preview, persistence, startup, and failure

Reuse Appearance's draft contributor. Preview is runtime-only. Save captures a
reference/revision, validates eligibility and commits the raw theme key. A failed storage write does not advance
savedTheme. If a later draft exists, restore its preview after committing the
captured choice. Reset/discard uses the last successful saved reference.

The initial implementation omits the optional shell-color cache. Only the saved
reference is persisted; authenticated inventory and validated content remain the
single palette source. This avoids stale custom colors on auth screens and keeps
startup storage failures harmless.

A Vite transformIndexHtml hook injects a synchronous pre-paint script. Built-ins
apply their scheme immediately; custom references initially use the system base
until authenticated inventory/content validation finishes. Auth change/logout
clears dynamic inventory and rendered package data, retaining only the device's
saved preference string. No package is fetched pre-auth. Plugins are permanently
available in this checkout; management retains the existing admin checks.

An active-theme invalidation falls back to base without changing storage. Show
localized status/retry and a direct Appearance recovery path. Plain-code fallback
uses effective foreground/background. Preserve the old visual result while a new
valid renderer theme compiles where possible, then publish the latest requested
digest; obsolete completions never win. Do not apply valid shell CSS from one
digest with permanently stale syntax colors from another without surfacing error.

Quick toggle/command actions keep their binary helpers; a custom dark theme's
Switch to Light Mode commits builtin light. Built-in recovery is always available
without uninstalling a pack. Package installation does not select it.

## Desktop and phone

Appearance uses the existing focused preferences page, SettingsRow and responsive
control sizing. Replace the fixed three-choice list with built-ins plus installed
variants, grouped by package; identify author/package on duplicate names. Include
Import and Browse themes links to the Themes marketplace. Non-admins see browse
and an explanation for restricted import. Contributed labels are inert author
product data; all host copy/errors/help use i18n.

Nearest exemplars: general-settings.tsx for selection and shared Save/Reset,
canvas-marketplace.tsx for list-to-detail, and the existing mobile full-height
review pattern. Appearance preserves its settings-content scroll owner and fixed
safe-area-clear save surface. Dense import report uses the separate plugin-owned
phone review surface described in the package design. The same selection/preview
state serves both widths; no phone-only persisted fallback. Full previews are
in the implementation plan.

## Requirement mapping and verification

| Requirement | Sections |
| --- | --- |
| REQ-UI-APP-THEMES-001 | Selection; Canonical tokens; Runtime renderer adapters |
| REQ-UI-APP-THEMES-002 | Selection; Preview, persistence, startup, and failure; Desktop and phone |

Unit/conformance tests cover unknown identifiers, role defaults, clearing previous
overrides, storage-failure/bootstrap behavior, digest-based invalidation, auth
changes, async races and error recovery. Integration mounts real settings drafts
and tests submitted-revision saves. Desktop/mobile E2E install a fixture pack and
import VS Code JSON, select both, exercise different renderers and undo/terminal
continuity, disable/remove/update selected variants, and recover to built-ins.
Do not treat a theme prop or a built-in monokai string as proof of imported-theme support.

## Primary compatibility evidence

- [Shiki custom theme API](https://shiki.style/guide/load-theme).
- [Monaco and TextMate distinction](https://shiki.style/packages/monaco).
- Pinned [Pierre theme registration](https://unpkg.com/@pierre/diffs@1.1.22/dist/highlighter/themes/registerCustomTheme.d.ts)
  and [worker update API](https://unpkg.com/@pierre/diffs@1.1.22/dist/worker/WorkerPoolManager.d.ts).
- Existing lockfile: Pierre 1.1.22, Shiki 3.22.0. Implement against installed
  exports; do not require upgrading to current upstream documentation versions.

## Implementation plan

- [Extensible theme support](../../../plans/application-themes/plan.md)
