import { type Page } from "@playwright/test";

import { test, expect } from "../../fixtures/test-base";

/**
 * Pseudo-locale coverage oracle.
 *
 * Under the pseudo-locale every EXTRACTED message is accented
 * (`Language` → `Ĺàńĝũàĝē`). So any user-facing copy that is still plain ASCII
 * is, by definition, a string that was never wrapped in a Lingui macro. This
 * spec crawls chrome-heavy screens and reports those leftovers.
 *
 * It runs TWO passes, because copy reaches the user by two routes:
 *   - visible text nodes, and
 *   - `COPY_ATTRIBUTES` — the copy a screen-reader user receives and a sighted
 *     one never sees. That pass is why the oracle is no longer blind to an
 *     `aria-label` that was never externalized; see docs/i18n.md.
 *
 * `SCREENS` mirrors `i18nGuardFiles` in eslint.i18n.options.mjs: a screen belongs
 * here once the components that render it have been migrated. The eslint guard
 * only sees plain literals in JSX, so it and this spec cover different halves of
 * the same question — add to both in the PR that migrates a directory.
 *
 * THIS IS A HARD CI GATE. It ran behind `KANDEV_I18N_COVERAGE=1` only while the
 * migration was mid-flight, because a screen's own copy could be clean while
 * shared chrome it renders was not yet. The last live directory has landed and
 * the env guard is gone: it now runs unconditionally in the `chromium` project
 * alongside every other spec.
 *
 *   pnpm e2e -- e2e/tests/i18n/pseudo-coverage.spec.ts
 *
 * WHAT IT GUARANTEES: on each screen in `SCREENS`, every rendered text node and
 * every value of a `COPY_ATTRIBUTES` attribute that the detector below can see
 * either came through `t()` / `<Trans>` (and so renders accented under the
 * pseudo-locale), or is on an allowlist that says why it must not be translated.
 *
 * That guarantee rests on the scan having run against the screen it names, which
 * for a while it did not. Each `SCREENS` entry carries a required `anchor` — a
 * selector that cannot match until that route rendered — and `waitForScreen`
 * blocks on it before anything is inspected. Read its comment before touching
 * the readiness logic: the wait it replaced was a one-second settle, and with
 * both lazy route chunks blocked so that NO screen rendered, 13-16 of the 21
 * tests still reported clean.
 *
 * It is a floor, not a proof of total coverage. Two limits are deliberate and
 * both are documented where they are implemented, because relaxing either
 * reddens every screen today:
 *   - `wordlike` requires a 4-letter ASCII run, so a hardcoded string shorter
 *     than that ("Add", "of") is not reported. Lowering it to 2 was measured:
 *     9 of the 10 tests here go red, on real but out-of-scope copy owned by
 *     `@kandev/ui` and sonner (e.g. the `alt+T` in sonner's toast-region label).
 *   - the text pass clears a node on ANY accented character, so an un-migrated
 *     word sharing one text node with migrated copy is missed. The attribute
 *     pass does NOT have this limit — it strips allowlisted tokens first and
 *     reports the remainder as `English frame, migrated value`. docs/i18n.md
 *     covers the text-node case under "…and it is weakest at an interpolated
 *     value"; it is why the by-hand pseudo walk is still step 9 of a migration.
 *
 * IF YOUR PR JUST WENT RED HERE, the failure names the exact strings and the
 * screen. Read them: each one is copy your change put on screen without routing
 * it through `t()`. This spec is the ONLY gate that can see most of them —
 * `i18next/no-literal-string` inspects literals in JSX and nothing else, so copy
 * in a variable, a default parameter, a `.ts` helper or a SCREAMING_CASE config
 * table is invisible to lint and visible only here. Repeated by-hand sweeps
 * found 15-60 such strings that every other gate passed.
 *
 * The fix is to externalize the string: add it to `src/locales/en/<ns>.json` and
 * call `t("<ns>:<key>")`. See docs/i18n.md. Adding an `allow:` entry is NOT the
 * fix and is the one failure mode this file has actually shipped twice — an
 * entry asserting a path was migrated while it still rendered English. `allow`
 * is for text the frontend must not translate but cannot avoid rendering (a
 * backend-owned record, a product name), and it must say where the value comes
 * from.
 *
 * BOTH passes walk `document.body`, deliberately, and NOT the page's `<main>`.
 * Scoping to the page under test is the obvious reading of what each test claims,
 * and it was tried and rejected on one fact: `@kandev/ui` builds Dialog,
 * AlertDialog, DropdownMenu, Tooltip and Select on Radix `Portal`, which mounts
 * to `document.body`. Every dialog, menu and tooltip in the app therefore renders
 * OUTSIDE `<main>`, so a scoped walk would stop checking the surfaces densest in
 * copy — and would do it silently, reporting clean. Trading a visible annoyance
 * for an invisible blind spot is the wrong direction for an oracle whose whole
 * purpose is the strings nothing else can see.
 *
 * The cost of body-wide is that a screen can fail for chrome it does not own.
 * That cost is now small — migrating the sidebar, settings nav and status bar
 * (#2214) took the baseline from 28 findings to 5 — and it buys the one thing
 * nothing else has: copy that belongs to no directory, like `@kandev/ui`'s
 * hardcoded breadcrumb landmark and sonner's toast-container label, is visible
 * only because the walk is body-wide. The fix for chrome noise is migrating
 * chrome, not narrowing the oracle.
 */

/**
 * Migrated screens whose visible text is overwhelmingly UI chrome, not user data.
 *
 * `allow` extends `ALLOWED` for one screen only. Use it for text the frontend
 * must NOT translate but cannot avoid rendering — records the backend owns, or a
 * product name — and say where the value comes from, so the exemption stays
 * auditable instead of quietly widening into a place to hide missed strings.
 *
 * `anchor` is REQUIRED and is what makes a green result mean anything; see
 * `waitForScreen` below for why a settle timeout did not.
 */
type Screen = {
  name: string;
  url: string;
  /**
   * A selector that cannot match until the ROUTE UNDER TEST has rendered. The
   * walk does not start until it matches AND the matched element's text is
   * accented.
   *
   * The test it must pass is not "is it unique" but "could the app shell alone
   * satisfy it?". `secrets-settings-body` also renders on the workspace-scoped
   * secrets page, and that is fine — reaching it still proves a secrets screen
   * mounted. What disqualifies a selector is matching something painted before
   * the route arrives, which is exactly the bug this field fixes.
   *
   * The two route trees answer that differently, and the difference is real
   * rather than an inconsistency to tidy up:
   *
   *   - SETTINGS screens anchor on a `data-testid` the page's own component
   *     renders. Seven already had one; `secrets-settings-body` and
   *     `sprites-connection-card` were added alongside this field.
   *   - OFFICE screens anchor on `main[data-office-route="<path>"]`, the route
   *     outlet in src/office-routes.tsx stamped with the RESOLVED path. A
   *     component-level testid was tried and rejected: the SPA route table
   *     mounts these pages with empty collections (`initialItems={[]}`), so they
   *     legitimately render empty states in e2e and any anchor inside the
   *     populated branch would never match. The outlet is present whichever
   *     branch a page takes, lives inside the office chunk, is absent while
   *     `OfficeRouteLoading` holds, and — because the selector pins the VALUE —
   *     cannot be satisfied by a different office route or by the unknown-route
   *     fallback.
   *
   * Note what carries the weight in the Office case: the element is generic, so
   * the ACCENTED half of `waitForScreen` is what proves a page actually painted
   * into it. An empty outlet has no accented text and times out.
   *
   * An attribute and not a heading name, deliberately, in both trees: the anchor
   * has to be findable BEFORE the pseudo catalog is known to have applied, and
   * every accessible name in the app changes under pseudo.
   */
  anchor: string;
  allow?: string[];
};

const SCREENS: Screen[] = [
  {
    name: "settings — appearance",
    url: "/settings/general/appearance",
    anchor: "[data-testid=theme-settings-card]",
  },
  {
    name: "settings — notifications",
    url: "/settings/general/notifications",
    anchor: "[data-testid=notification-sound-group]",
    // Provider names are rows in the notification_providers table. The backend
    // seeds these two (apps/backend/internal/notifications/service/service.go)
    // and users name their own Apprise ones, so they are data on the same
    // footing as a task title. `Apprise` labels the provider type.
    allow: ["Desktop Notifications", "System Notifications", "Apprise"],
  },
  {
    name: "settings — secrets",
    url: "/settings/general/secrets",
    anchor: "[data-testid=secrets-settings-body]",
  },
  {
    name: "settings — terminal",
    url: "/settings/general/terminal",
    anchor: "[data-testid=terminal-font-size-card]",
  },
  {
    name: "settings — sprites",
    url: "/settings/general/sprites",
    anchor: "[data-testid=sprites-connection-card]",
    // Product name of the sandbox provider.
    allow: ["Sprites.dev"],
  },
  {
    name: "settings — layouts",
    url: "/settings/general/layouts",
    anchor: "[data-testid=layout-settings]",
    // NOTE: everything here must be a PERSISTED name. This list is load-bearing
    // in the wrong direction — a broad token also hides genuinely un-migrated
    // copy that happens to match it. "Default" already masked the
    // default-action button once, which rendered a raw English "Default" until
    // `getDefaultActionState` was fixed to resolve it through the catalog.
    //
    // It fails in the OTHER direction too, and has: these entries duplicate
    // strings that live in `lib/layout/layout-profiles.ts`, with nothing keeping
    // the two in sync. #2198 changed the Default profile's description and this
    // list kept the old wording, so it manufactured a phantom finding for every
    // run between `cc6eb4dd5` and this commit. Re-check these against
    // `BUILT_IN_LAYOUT_PROFILES` whenever a built-in name or description moves.
    //
    // Both groups are display strings that are also PERSISTED, so translating
    // them in place would write locale-dependent values into a user's saved
    // layouts and leave them there after a locale switch:
    //   - Built-in profile names/descriptions (lib/layout/layout-profiles.ts).
    //     `upsertBuiltInLayoutOverride` copies `builtIn.name` into the saved
    //     record the first time a built-in is customized.
    //   - Dockview panel titles (lib/state/layout-manager/constants.ts), which
    //     `toSerializedDockview` writes into the stored layout JSON. That path
    //     is already in `EXCLUDED` in scripts/externalize-strings.mjs.
    // Localizing either needs a key/persisted-value split in those modules.
    allow: [
      "Default",
      "Plan Mode",
      "Preview Mode",
      "VS Code",
      "Agent with Files, Changes, and Terminal",
      "Agent and Plan side by side",
      "Agent and Browser side by side",
      "Agent and VS Code side by side",
      "Agent",
      "Plan",
      "Changes",
      "Files",
      "Browser",
      "Terminal",
      "PR Details",
      "Merge Request",
    ],
  },
  {
    name: "settings — keyboard shortcuts",
    url: "/settings/general/keyboard-shortcuts",
    anchor: "[data-testid=chat-submit-key-card]",
    // Modifier/key names label a physical key and are out of scope for
    // translation — the same rule the eslint guard's keyboard pattern encodes.
    //
    // Shortcut names come from CONFIGURABLE_SHORTCUTS in
    // lib/keyboard/shortcut-overrides.ts, a registry shared with the
    // un-migrated voice-mode settings page; it migrates with that page.
    allow: [
      "Ctrl",
      "Shift",
      "Alt",
      "Cmd",
      "Meta",
      "Space",
      "Enter",
      "Tab",
      "Command Panel",
      "Command Panel (Alt)",
      "File Search",
      "Search Task Contents",
      "Quick Chat",
      "Toggle Bottom Terminal",
      "Toggle Sidebar",
      "New Task",
      "Focus Chat Input",
      "Focus CLI Chat Input",
      "Toggle Plan Mode",
      "Recent Task Switcher",
      "Recent Task Switcher (Backward)",
      "Voice Input",
      "Reverse Chat Search",
      "Open Task Pull Request",
    ],
  },
  {
    name: "settings — task actions",
    url: "/settings/general/task-actions",
    anchor: "[data-testid=archive-confirmation-card]",
  },
  {
    name: "settings — plugins",
    url: "/settings/plugins",
    anchor: "[data-testid=plugins-tab-installed]",
  },
  // Office — the last live directory to migrate (#2357 was the batch before it).
  // All twelve routes were run under the pseudo-locale before being listed here;
  // eleven came back with nothing, and the one `allow` below is the only string
  // any of them renders that must stay ASCII.
  //
  // These twelve reach the browser through a DIFFERENT route tree than the
  // settings screens above — their own lazy chunk, loaded by `office-routes`
  // rather than `settings-routes` — so they need their own anchors and were
  // proved separately; see `waitForScreen`.
  // NOT YET: "office — dashboard" (`/office`). The entry existed and passed, and
  // it was not testing the dashboard.
  //
  // `/office` renders the dashboard only for a workspace that has an
  // `office_workflow_id` (`hasOfficeWorkspace` in src/office-routes.tsx). The
  // shared e2e fixture never seeds one, so `resolveOfficeHomeSetupRedirect`
  // sends `/office` to `/office/setup?mode=new` on every run, and the screen the
  // walk actually scanned was the SETUP WIZARD — "Set up your Office workspace",
  // a different screen from the one the test named. It reported clean because
  // the wizard is itself fully migrated, so the pass was real and was about
  // something else.
  //
  // The anchor is what surfaced this: the redirect is invisible to a walk that
  // only asks "is anything accented". Restoring real dashboard coverage needs an
  // office workflow seeded in the fixture, which is a change to shared
  // `test-base` state and belongs with whoever owns that fixture — not an
  // allowlist entry and not a renamed screen, either of which would keep the
  // false claim alive in a new form.
  {
    name: "office — inbox",
    url: "/office/inbox",
    anchor: 'main[data-office-route="/office/inbox"]',
  },
  {
    name: "office — tasks",
    url: "/office/tasks",
    anchor: 'main[data-office-route="/office/tasks"]',
  },
  {
    name: "office — agents",
    url: "/office/agents",
    anchor: 'main[data-office-route="/office/agents"]',
  },
  {
    name: "office — projects",
    url: "/office/projects",
    anchor: 'main[data-office-route="/office/projects"]',
  },
  {
    name: "office — routines",
    url: "/office/routines",
    anchor: 'main[data-office-route="/office/routines"]',
  },
  {
    name: "office — workspace costs",
    url: "/office/workspace/costs",
    anchor: 'main[data-office-route="/office/workspace/costs"]',
  },
  {
    name: "office — workspace activity",
    url: "/office/workspace/activity",
    anchor: 'main[data-office-route="/office/workspace/activity"]',
  },
  {
    name: "office — workspace routing",
    url: "/office/workspace/routing",
    anchor: 'main[data-office-route="/office/workspace/routing"]',
  },
  {
    name: "office — workspace skills",
    url: "/office/workspace/skills",
    anchor: 'main[data-office-route="/office/workspace/skills"]',
  },
  {
    name: "office — workspace org",
    url: "/office/workspace/org",
    anchor: 'main[data-office-route="/office/workspace/org"]',
  },
  {
    name: "office — workspace settings",
    url: "/office/workspace/settings",
    anchor: 'main[data-office-route="/office/workspace/settings"]',
    // The clone field's placeholder is an example git URL — the SHAPE the user
    // is meant to imitate, on the same footing as the email and CSS-function
    // placeholders the eslint guard already excludes via its `(https?|ssh|git)://`
    // pattern. Accenting it would show a URL that cannot be typed.
    allow: ["https://github.com/org/config.git"],
  },
  // NOT YET: "settings — executors", "settings — account security",
  // "settings — account tokens". All three routes' own copy is fully migrated
  // and was verified by walking them under pseudo — including every dialog and
  // both marketplace tabs, since a static page scan never opens a Radix portal.
  // What stops them being entries here is what the fixture renders BESIDE that
  // copy, and in each case an `allow` entry would be the wrong fix:
  //   - `/settings/executors` renders the seeded executor profiles' NAMES, and
  //     the e2e fixture happens to name them `Local` and `Worktree`. That is
  //     user data on exactly the footing as the workspace names below, so it
  //     needs `findUnlocalizedText` to stop treating user data as eligible —
  //     listing the two values would fix this fixture and leave every developer
  //     instance broken under different names. The hub cards' own `Docker` and
  //     `Sprites.dev` labels are brand nouns; they are in the guard's
  //     `words.exclude` but not in `ALLOWED`, which is a real sync gap in that
  //     list rather than something this migration should widen it to paper over.
  //   - Both account routes render `not found` — the router's 404 body, because
  //     the e2e profile does not register the auth routes, surfaced in the
  //     sessions/tokens error region. It is a backend diagnostic, English by
  //     design (docs/i18n.md, "What the backend deliberately does NOT
  //     translate"), and allowlisting a payload string would hide the next real
  //     miss in the same region.
  // The `plugins` entry above has neither problem and passes with no `allow`.
  // Its marketplace entries — names, descriptions, categories, source names and
  // index.json URLs — come from the catalog rather than the DOM on load.
  //
  // NOT YET: "settings — integrations github", "… gitlab", "… jira",
  // "… linear", "… sentry". Each page's
  // own copy is fully migrated (verified by running this oracle against it —
  // every string the integration owns renders accented).
  //
  // The settings-nav half of this blocker is now GONE: `settings-tree.tsx`,
  // `workspaces-group.tsx`, `executors-group.tsx`, `account-group.tsx` and
  // `theme-toggle.tsx` are migrated, so "Workspaces", "Integrations",
  // "Automations", "Executors", "Voice Mode", "Utility Agents", "External MCP",
  // "Plugins" and "Toggle theme" render accented on every screen. Only "System"
  // is left, from `sections/settings/system-group.tsx`, which the System-routes
  // migration owns.
  //
  // What still blocks these five entries is the shared integration chrome, not
  // the nav. `components/integrations/**` is now migrated, which cleared
  // `drafted-integration-enabled-control.tsx` ("Enabled"/"Disabled"),
  // `auth-status-banner.tsx` ("Authenticated", "· checked <relative>") and the
  // watcher card's loading state. What is left is
  // `components/watcher-repository-fields.tsx` ("Repository", "Base Branch",
  // "(no repository)"), `STEP_DEFAULT_LABEL` and `stepPlaceholder` — all shared
  // with the un-migrated Azure DevOps surface — plus
  // `components/integrations/settings-section.tsx` chrome and `@kandev/ui`'s
  // built-in dialog "Close" label.
  //
  // NOT YET: "settings — external mcp", "… prompts", "… voice mode",
  // "… utility agents". All four pages' own copy is fully migrated and was
  // verified by running this oracle against each of them — every string those
  // routes own renders accented, including the 43 in the MCP tool catalog
  // (`lib/settings/external-mcp-tools.ts`), which is a `.ts` module no lint rule
  // inspects. The collapsed tools preview, the utility agent dialog and the
  // inference status note were scanned separately, since this spec only sees
  // what a route renders on load.
  //
  // #2214 migrated the settings nav, which was the original blocker, and these
  // four now come back clean of it. What still stops them being entries here is
  // copy none of them owns:
  //   - `aria-label="breadcrumb"` from `@kandev/ui`'s Breadcrumb primitive, and
  //     Sonner's `Notifications alt+T` toast-region label. Both are the shared-UI
  //     case docs/i18n.md describes: they need a strings-provider seam in the
  //     package, not a per-route fix.
  //   - `Configuration Chat` from `components/config-chat/`, not yet migrated.
  // Allowlisting either here would hide misses belonging to whoever owns them,
  // which is the failure this list's own comment warns about. The workspace
  // names these routes also surfaced are no longer among them: #2220 derives
  // them from the boot payload and makes them ineligible, which is the right
  // fix — they are user data, and listing them by value would have fixed the
  // fixture and left every developer's own instance broken.
  //
  // Each route also renders data that is correctly English and would need an
  // `allow` entry: the built-in prompts' names and bodies and the built-in
  // utility agents' names and descriptions (both backend-authored), the agent
  // product names and config paths on External MCP, and `Ctrl+Shift+M`.
  //
  // NOT YET: "settings — workspace workflows"
  // (`/settings/workspace/:id/workflows`). The workflow editor's own copy is
  // fully migrated and was verified with this oracle against a live instance,
  // and the Workspaces branch of the nav it expands is now migrated too. What
  // remains is that the route renders the workspace's own name and its
  // workflow/step names — user data, on the same footing as the fixture's
  // "E2E Workspace". Adding the entry needs `findUnlocalizedText` to stop
  // treating user data as eligible, not an allowlist entry per fixture name.
];

/**
 * Text that is legitimately un-accented under pseudo: brand/proper nouns, code
 * identifiers, and units/symbols. Kept in sync with `words.exclude` in
 * apps/web/eslint.i18n.options.mjs.
 */
const ALLOWED = [
  "Kandev",
  "GitHub",
  "GitLab",
  "Jira",
  "Linear",
  "Slack",
  "Sentry",
  "Azure DevOps",
  "ACP",
  "MCP",
  "SSH",
  "URL",
  "ID",
  "English",
  "Pseudo",
  "QA",
];

/**
 * Any character in the accented range the pseudo-locale transform emits. Shared
 * with the in-page detector below, which cannot close over it.
 */
const ACCENTED = /[À-ɏ]/;

async function activatePseudo(page: Page, url: string) {
  await page.goto(url);
  await page.evaluate(() => {
    document.cookie = "kandev_locale=pseudo; path=/; max-age=31536000; SameSite=Lax";
  });
  await page.reload();
  // `<html lang>` proves the SHELL is on the pseudo locale and nothing more: the
  // Go shell writes it from the cookie, so it is already correct in the HTML the
  // browser parses, before any script has run. Every stronger fact — the route
  // rendered, and the catalog reached its copy — is `waitForScreen`'s job, and
  // it is per-screen precisely because anything generic enough to assert here is
  // something the app shell alone satisfies.
  await expect(page.locator("html")).toHaveAttribute("lang", "pseudo", { timeout: 15_000 });
}

/**
 * Block until the SCREEN UNDER TEST has rendered under the pseudo locale.
 *
 * This replaces a `waitForTimeout(1_000)` settle, and the reason is the only
 * thing in this file worth reading twice: THE SETTLE LET THE ORACLE REPORT A
 * PASS IT HAD NOT EARNED, and it did so in the direction that hurts.
 *
 * `SCREENS` are lazy routes in two separate chunks — `settings-routes` (2.8 MB)
 * and `office-routes`. Nothing in the old wait was tied to either arriving:
 *   - `activatePseudo`'s `lang="pseudo"` assertion is written server-side by
 *     shell.go before a single script runs, so it is true immediately;
 *   - the `inspectedAttributes > 0` control below is satisfied by the sidebar,
 *     topbar and status bar, which are accented long before the route renders;
 *   - so the only thing standing between "route rendered" and "scan the DOM"
 *     was one second of wall clock.
 * Measured with BOTH chunks blocked outright, so that not one screen rendered:
 * 13-16 of the 21 tests reported clean, varying run to run — because whether a
 * screen "passed" came down to a race the settle could not see. The failure mode
 * is worse than flakiness — under CI load a slow render makes a green result
 * MORE likely, so the gate is at its most permissive exactly when the tree is
 * most likely to be broken. With the anchors below the same probe fails all 21,
 * every run.
 *
 * The fix has to be per-screen. Any signal generic enough to share is a signal
 * the app shell already satisfies, which is how this happened in the first
 * place. So each screen names a selector nothing but that route can satisfy, and
 * this waits on two facts about it:
 *   1. it is VISIBLE — the lazy route mounted and this screen, specifically, is
 *      the one on screen;
 *   2. its text is ACCENTED — the pseudo catalog has actually been applied to
 *      copy this route owns. This was written as insurance against a catalog
 *      that arrives asynchronously; it stopped being hypothetical the moment
 *      lazy catalog loading landed, and `pseudo` is now a fetched chunk like
 *      every locale but `en`. Without it the walk could run against a route
 *      rendered in English fallback and report every string on it as a miss.
 *      Waiting here is robust to that without weakening anything: a wait can
 *      only delay the scan, never excuse a finding.
 *
 * Both are `expect` assertions, not `waitFor`s, so an anchor that never arrives
 * FAILS this spec instead of quietly scanning whatever the shell had painted.
 * That is the whole point: the gate has to be capable of going red.
 *
 * Do NOT be tempted to swap this for `waitForLoadState("networkidle")`. Kandev
 * holds a live WebSocket for the session stream, so the network is never idle
 * and it would hang until timeout — it looks like a stronger wait and is not a
 * wait at all.
 */
async function waitForScreen(page: Page, screen: Screen) {
  // NOT `.first()`. A selector that matches more than one element is an
  // ambiguous anchor, and `.first()` would silently pick one and gate on it —
  // the same shape of mistake as the settle this function replaced. Playwright's
  // strict mode turns that ambiguity into a failure, which is the direction this
  // spec wants. All nine anchors resolve to exactly one element today.
  const anchor = page.locator(screen.anchor);

  await expect(
    anchor,
    `The render anchor \`${screen.anchor}\` never appeared on ${screen.name}, so the ` +
      `route under test did not render. Scanning now would report the app shell as ` +
      `clean and prove nothing about ${screen.name}. If this element was renamed, ` +
      `update the \`anchor\` for this screen in SCREENS.`,
  ).toBeVisible({ timeout: 15_000 });

  await expect(
    anchor,
    `The render anchor \`${screen.anchor}\` rendered on ${screen.name} but its copy ` +
      `never went accented, so the pseudo catalog had not been applied to this ` +
      `route's copy. Every string on the screen would be reported as a miss. If ` +
      `this anchor's own copy was just un-externalized, THAT is the finding — fix ` +
      `it rather than moving the anchor.`,
  ).toHaveText(ACCENTED, { timeout: 15_000 });
}

/**
 * Attributes that carry display copy rather than a value the app compares.
 * Kept in sync with `jsx-attributes.include` in apps/web/eslint.i18n.options.mjs,
 * so the guard and this oracle answer the same question about the same set: on an
 * allowlisted path lint flags the literal, and here it is checked at render.
 *
 * `aria-labelledby` / `aria-describedby` / `aria-controls` are deliberately
 * absent — they carry element ids, not prose, and are excluded by the guard too.
 *
 * `title` is checked on EVERY element, not just interactive ones. A `title` is
 * exposed as an accessible name or description whatever the element is, and the
 * browser renders its native tooltip on hover for any of them — so "the app never
 * shows this one" is not true of a rendered `title`. Restricting it to
 * interactive elements would need a tag/role/tabindex heuristic, and that
 * heuristic would immediately become the place attribute copy hides. The guard
 * checks `title` unconditionally too; the two stay in step.
 */
const COPY_ATTRIBUTES = ["aria-label", "aria-description", "title", "placeholder", "alt"];

/**
 * Collect copy that looks un-externalized: plain-English visible text nodes, and
 * plain-English values of the five copy-bearing attributes.
 *
 * Attribute findings are returned prefixed with the attribute and the element
 * that carries it (`aria-label on button[data-testid=…]: "More information"`),
 * because an attribute string is invisible on screen — a bare value gives you
 * nothing to grep for and nowhere to look.
 *
 * Two counters make an empty `leftovers` mean something, because a selector that
 * matched no elements reports exactly as clean as a clean screen:
 *   - `inspectedAttributes` — non-empty attribute values examined. Asserted per
 *     screen; this is the one that catches a selector matching nothing.
 *   - `localizedAttributes` — those that rendered FULLY accented, i.e. migrated
 *     attribute copy the pass demonstrably reached. Not asserted per screen: a
 *     page of plain inputs and text can legitimately have none, and secrets,
 *     terminal and sprites do. Pinned once instead, in its own test below.
 */
async function findUnlocalizedCopy(
  page: Page,
  allowed: string[],
): Promise<{ leftovers: string[]; localizedAttributes: string[]; inspectedAttributes: number }> {
  return page.evaluate(
    ({ allowedList, copyAttributes }) => {
      const skipTags = new Set(["SCRIPT", "STYLE", "NOSCRIPT", "CODE", "PRE", "SVG"]);
      // A narrower set for attributes. `skipTags` above encodes "this element's
      // CONTENT is not prose", which is a different question from whether its
      // LABEL is: an `aria-label` on an <svg> icon, or a `title` on a <code>
      // chip, is copy the user receives either way. Only the three tags that
      // cannot present a label at all are skipped here.
      const attrSkipTags = new Set(["SCRIPT", "STYLE", "NOSCRIPT"]);
      const wordlike = /[A-Za-z]{4,}/;
      // Same range as `ACCENTED` above, restated because this body is
      // serialized into the page and cannot close over module scope.
      const accented = /[À-ɏ]/;

      // User data is never copy, and a workspace's name is user data on the same
      // footing as a task title. Derived from the boot payload rather than
      // listed, because listing it fixes one instance and leaves every other
      // one broken: `E2E Workspace` is only the fixture's name, and a developer
      // running this against their own instance would have hit the identical
      // false positive under a different string.
      const workspaceNames = (() => {
        const payload = (window as unknown as { __KANDEV_BOOT_PAYLOAD__?: unknown })
          .__KANDEV_BOOT_PAYLOAD__;
        const state = (payload as { initialState?: { workspaces?: { items?: unknown } } })
          ?.initialState;
        const items = state?.workspaces?.items;
        if (!Array.isArray(items)) return [];
        return items
          .map((item) => (item as { name?: unknown })?.name)
          .filter((name): name is string => typeof name === "string" && name.trim().length > 0);
      })();

      // Longest first: stripping "VS Code" before "Agent and VS Code side by
      // side" would leave "side by side" behind and report it as a leftover.
      const tokens = [...allowedList, ...workspaceNames].sort((a, b) => b.length - a.length);

      /** Still word-like ASCII once allowlisted tokens are removed. */
      const hasUnmigratedAscii = (text: string) => {
        if (!text || !wordlike.test(text)) return false;
        let residue = text;
        for (const token of tokens) residue = residue.split(token).join(" ");
        return wordlike.test(residue);
      };

      /** The text pass's rule, unchanged: any accented character clears a node. */
      const looksUnlocalized = (text: string) => !accented.test(text) && hasUnmigratedAscii(text);

      // The text pass drops zero-size nodes because nothing is on screen to read.
      // That test is WRONG for an attribute: a visually hidden control with an
      // `aria-label` is precisely the case this pass exists to catch, and every
      // `sr-only` node measures as good as zero. What actually disqualifies a
      // label is the element not being rendered at all — `display: none`,
      // `visibility: hidden`, or a skipped `content-visibility` subtree — because
      // then no user receives it, sighted or not. `checkVisibility` asks that
      // directly; deliberately WITHOUT `opacityProperty`, since an opacity-0
      // element is still in the accessibility tree.
      const isRendered = (el: Element) =>
        typeof el.checkVisibility !== "function" ||
        el.checkVisibility({ contentVisibilityAuto: true, visibilityProperty: true });

      const collectText = () => {
        const found = new Set<string>();
        const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
        let node: Node | null;
        while ((node = walker.nextNode())) {
          const text = (node.textContent ?? "").trim();
          if (!looksUnlocalized(text)) continue;

          const el = node.parentElement;
          if (!el || skipTags.has(el.tagName)) continue;
          // Same rendered-or-not test the attribute pass uses, rather than the
          // zero-size rect this used to apply. The rect check kept `sr-only`
          // text ONLY because that utility measures 1px rather than 0 — so
          // whether screen-reader-only copy was checked came down to a CSS
          // implementation detail, and changing `.sr-only` to `clip` at 0×0
          // would have silently dropped it. It IS copy, and a deliberately
          // in-scope kind: `theme-toggle.tsx`'s "Toggle theme" reached a user
          // only this way. `checkVisibility` keeps it for the right reason —
          // the element renders and is in the accessibility tree — while
          // correctly dropping `visibility: hidden`, which the rect check kept.
          if (!isRendered(el)) continue;

          found.add(text.slice(0, 120));
        }
        return [...found];
      };

      /** Enough to find the element in the DOM and in the source. */
      const describe = (el: Element) => {
        const tag = el.tagName.toLowerCase();
        const testId = el.getAttribute("data-testid");
        if (testId) return `${tag}[data-testid=${testId}]`;
        if (el.id) return `${tag}#${el.id}`;
        const role = el.getAttribute("role");
        return role ? `${tag}[role=${role}]` : tag;
      };

      const seen = new Map<string, { label: string; count: number }>();
      // The positive control. An attribute that HAS been migrated renders as
      // `Mōŕē ĩńfōŕmàţĩōń`, which contains no 4-letter ASCII run — so it is
      // dropped at the `wordlike` gate and never even reaches the `accented`
      // test. Soundness is unaffected (a real miss is pure ASCII and is still
      // caught), but it means a working pass and a pass whose selector matched
      // NOTHING produce identical empty output. Collecting the accented values
      // proves the pass actually reached migrated attribute copy on this
      // screen, so the caller can tell those two apart.
      const localized: string[] = [];
      let inspected = 0;

      const inspectAttribute = (el: Element, attr: string) => {
        // An empty value is never a missed string, and `alt=""` is load-bearing:
        // it marks an image as decorative so screen readers skip it. Reporting
        // one would push authors into writing alt text for spacers.
        const value = (el.getAttribute(attr) ?? "").trim();
        if (!value) return;
        inspected += 1;

        // NOTE the divergence from the text pass, which clears a node the moment
        // it contains one accented character. That rule hides a real shape here:
        // an `aria-label` built as "Collapse ${label}" renders
        // "Collapse Ĝēńēŕàĺ" — an un-migrated English FRAME around a migrated
        // value, which accent-clears-all would call done. Allowlist stripping is
        // what separates it from the legitimate case, where the ASCII fragment is
        // the allowlisted DATA rather than the frame ("Àćţĩōńś ƒōŕ Agent" on the
        // layouts screen strips to nothing).
        if (!hasUnmigratedAscii(value)) {
          // Fully accented, nothing ASCII left: a migrated attribute, and the
          // positive control.
          if (accented.test(value)) {
            localized.push(`${attr} on ${describe(el)}: "${value.slice(0, 60)}"`);
          }
          return;
        }

        // Keyed by attribute+value, not by element: one un-migrated shared
        // component renders on twenty rows, and twenty identical findings bury
        // the other nineteen strings. The first element carrying it is the
        // sample you go and look at.
        const key = `${attr} ${value}`;
        const hit = seen.get(key);
        if (hit) {
          hit.count += 1;
          return;
        }
        const label = `${attr} on ${describe(el)}: "${value.slice(0, 120)}"`;
        seen.set(key, {
          label: accented.test(value) ? `${label} - English frame, migrated value` : label,
          count: 1,
        });
      };

      const collectAttributes = () => {
        const selector = copyAttributes.map((attr) => `[${attr}]`).join(",");
        for (const el of Array.from(document.querySelectorAll(selector))) {
          if (attrSkipTags.has(el.tagName.toUpperCase()) || !isRendered(el)) continue;
          for (const attr of copyAttributes) inspectAttribute(el, attr);
        }
        const leftovers = [...seen.values()].map(({ label, count }) =>
          count > 1 ? `${label} (×${count})` : label,
        );
        return { leftovers, localized, inspected };
      };

      const attributes = collectAttributes();
      return {
        leftovers: [...collectText(), ...attributes.leftovers],
        localizedAttributes: attributes.localized,
        inspectedAttributes: attributes.inspected,
      };
    },
    { allowedList: allowed, copyAttributes: COPY_ATTRIBUTES },
  );
}

test.describe("i18n pseudo-locale coverage", () => {
  for (const screen of SCREENS) {
    test(`no un-externalized copy on ${screen.name}`, async ({ testPage }) => {
      await activatePseudo(testPage, screen.url);
      await waitForScreen(testPage, screen);

      const { leftovers, localizedAttributes, inspectedAttributes } = await findUnlocalizedCopy(
        testPage,
        [...ALLOWED, ...(screen.allow ?? [])],
      );

      // Control FIRST. A migrated attribute renders accented and so is invisible
      // to the leftover check by construction, which means a screen that is
      // clean and a selector that matched NOTHING report identically. Assert the
      // pass actually read attributes before believing what it did not find.
      //
      // NOTE what this does and does not establish, because getting that wrong
      // is what made this spec passable without looking at anything: it proves
      // the pass READ ATTRIBUTES, not that it read THIS SCREEN'S. The sidebar,
      // topbar and status bar supply well over a hundred of them on every route,
      // so this stays green on a page where the route never mounted. `anchor`
      // above is what ties the scan to the screen under test; this remains
      // useful only as the narrower check that the selector itself works.
      expect(
        inspectedAttributes,
        `The attribute pass examined no attribute at all on ${screen.name}, so a ` +
          `green leftover check proves nothing about attribute copy here.`,
      ).toBeGreaterThan(0);

      expect(
        leftovers,
        `Un-externalized strings on ${screen.name}:\n${leftovers.map((s) => `  - ${s}`).join("\n")}` +
          `\n\nAttribute pass examined ${inspectedAttributes} attribute(s), of which ` +
          `${localizedAttributes.length} rendered fully accented.`,
      ).toEqual([]);
    });
  }

  /**
   * The positive control, pinned once rather than per screen.
   *
   * `inspectedAttributes` above proves the selector matched; it does not prove
   * the pass can tell migrated attribute copy apart from a miss. This does, on
   * the one element we know is migrated: `language-settings.tsx` renders
   * `aria-label={t("settings:displayLanguage")}` on `#language-select`, so under
   * pseudo it must come back accented. If this test fails while the screens
   * above pass, the accent detection is broken and every green screen is
   * meaningless — which is the failure this whole file exists to not have.
   *
   * A per-screen version of this assertion was tried and dropped: secrets,
   * terminal and sprites legitimately render no fully-accented attribute at all,
   * so it fired on three screens that were fine.
   */
  test("the attribute pass recognizes migrated attribute copy", async ({ testPage }) => {
    const appearance = SCREENS.find((screen) => screen.url === "/settings/general/appearance");
    if (!appearance) throw new Error("the appearance screen is no longer in SCREENS");

    await activatePseudo(testPage, appearance.url);
    await waitForScreen(testPage, appearance);

    const { localizedAttributes } = await findUnlocalizedCopy(testPage, ALLOWED);

    expect(
      localizedAttributes.join("\n"),
      `Expected the migrated aria-label on #language-select to render accented. Got:\n` +
        localizedAttributes.join("\n"),
    ).toContain("language-select");
  });
});
