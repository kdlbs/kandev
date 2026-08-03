import { type Page } from "@playwright/test";

import { test, expect } from "../../fixtures/test-base";

/**
 * Pseudo-locale coverage oracle.
 *
 * Under the pseudo-locale every EXTRACTED message is accented
 * (`Language` → `Ĺàńĝũàĝē`). So any user-facing text that is still plain ASCII
 * is, by definition, a string that was never wrapped in a Lingui macro. This
 * spec crawls chrome-heavy screens and reports those leftovers.
 *
 * `SCREENS` mirrors `i18nGuardFiles` in eslint.i18n.options.mjs: a screen belongs
 * here once the components that render it have been migrated. The eslint guard
 * only sees plain literals in JSX, so it and this spec cover different halves of
 * the same question — add to both in the PR that migrates a directory.
 *
 * Gated on `KANDEV_I18N_COVERAGE=1` rather than run in CI: with the migration
 * proceeding one directory at a time, a screen's own copy can be clean while
 * shared chrome it renders is not yet. It becomes a hard gate when the last
 * directory lands and the env guard comes off.
 *
 *   KANDEV_I18N_COVERAGE=1 pnpm e2e -- e2e/tests/i18n/pseudo-coverage.spec.ts
 */

const COVERAGE_ENABLED = process.env.KANDEV_I18N_COVERAGE === "1";

/**
 * Migrated screens whose visible text is overwhelmingly UI chrome, not user data.
 *
 * `allow` extends `ALLOWED` for one screen only. Use it for text the frontend
 * must NOT translate but cannot avoid rendering — records the backend owns, or a
 * product name — and say where the value comes from, so the exemption stays
 * auditable instead of quietly widening into a place to hide missed strings.
 */
const SCREENS: Array<{ name: string; url: string; allow?: string[] }> = [
  { name: "settings — appearance", url: "/settings/general/appearance" },
  {
    name: "settings — notifications",
    url: "/settings/general/notifications",
    // Provider names are rows in the notification_providers table. The backend
    // seeds these two (apps/backend/internal/notifications/service/service.go)
    // and users name their own Apprise ones, so they are data on the same
    // footing as a task title. `Apprise` labels the provider type.
    allow: ["Desktop Notifications", "System Notifications", "Apprise"],
  },
  { name: "settings — secrets", url: "/settings/general/secrets" },
  { name: "settings — terminal", url: "/settings/general/terminal" },
  {
    name: "settings — sprites",
    url: "/settings/general/sprites",
    // Product name of the sandbox provider.
    allow: ["Sprites.dev"],
  },
  {
    name: "settings — layouts",
    url: "/settings/general/layouts",
    // NOTE: everything here must be a PERSISTED name. This list is load-bearing
    // in the wrong direction — a broad token also hides genuinely un-migrated
    // copy that happens to match it. "Default" already masked the
    // default-action button once, which rendered a raw English "Default" until
    // `getDefaultActionState` was fixed to resolve it through the catalog.
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
      "Agent with Files, Changes, PR Details, and Terminal",
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
  { name: "settings — task actions", url: "/settings/general/task-actions" },
  // NOT YET: "settings — integrations github", "… gitlab", "… jira",
  // "… linear", "… sentry". Each page's
  // own copy is fully migrated (verified by running this oracle against it —
  // every string the integration owns renders accented), but the route expands
  // the Workspaces > Integrations branch of the settings nav, and
  // `workspaces-group.tsx` / `settings-tree.tsx` are not migrated: "Workspaces",
  // "Integrations", "Automations", "Executors", "Voice Mode", "Utility Agents",
  // "External MCP", "Plugins", "System" and "Toggle theme" all still render plain
  // English there. The /settings/general/* screens above pass because their
  // expanded branch is `general-group.tsx`, which is migrated. Add these entries
  // in the PR that migrates the settings nav — allowlisting those nav labels here
  // would hide real misses instead.
  //
  // Two shared components rendered by all five pages are also still English and
  // would have to be migrated (or allowlisted, which is worse) first:
  // `components/integrations/drafted-integration-enabled-control.tsx`
  // ("Enabled"/"Disabled") and `components/watcher-repository-fields.tsx`
  // ("Repository", "Base Branch", "(no repository)"), plus `STEP_DEFAULT_LABEL`
  // and `stepPlaceholder`. All four are shared with the un-migrated Azure DevOps
  // surface, plus `components/integrations/settings-section.tsx` chrome and the
  // watcher card's own empty/loading states.
  //
  // The Sentry run also surfaced `components/integrations/auth-status-banner.tsx`
  // ("Authenticated", "· checked <relative>") and `@kandev/ui`'s built-in dialog
  // "Close" label — both shared, both out of scope for a per-integration PR.
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

async function activatePseudo(page: Page, url: string) {
  await page.goto(url);
  await page.evaluate(() => {
    document.cookie = "kandev_locale=pseudo; path=/; max-age=31536000; SameSite=Lax";
  });
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("lang", "pseudo", { timeout: 15_000 });
}

/**
 * Collect visible text nodes that look like un-externalized English copy.
 * Nodes inside script/style, and nodes that are wholly allowlisted, are ignored.
 */
async function findUnlocalizedText(page: Page, allowed: string[]): Promise<string[]> {
  return page.evaluate((allowedList) => {
    const skipTags = new Set(["SCRIPT", "STYLE", "NOSCRIPT", "CODE", "PRE", "SVG"]);
    const wordlike = /[A-Za-z]{4,}/;
    const accented = /[À-ɏ]/;
    const found = new Set<string>();

    const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
    let node: Node | null;
    while ((node = walker.nextNode())) {
      const text = (node.textContent ?? "").trim();
      if (!text || !wordlike.test(text) || accented.test(text)) continue;

      const el = node.parentElement;
      if (!el || skipTags.has(el.tagName)) continue;
      // Ignore hidden nodes — not user-visible copy.
      const rect = el.getBoundingClientRect();
      if (rect.width === 0 && rect.height === 0) continue;

      // Strip allowlisted tokens; if nothing word-like remains, it's fine.
      // Longest first: stripping "VS Code" before "Agent and VS Code side by
      // side" would leave "side by side" behind and report it as a leftover.
      let residue = text;
      for (const token of [...allowedList].sort((a, b) => b.length - a.length)) {
        residue = residue.split(token).join(" ");
      }
      if (!wordlike.test(residue)) continue;

      found.add(text.slice(0, 120));
    }
    return [...found];
  }, allowed);
}

test.describe("i18n pseudo-locale coverage", () => {
  test.skip(
    !COVERAGE_ENABLED,
    "Set KANDEV_I18N_COVERAGE=1 to run the string-externalization oracle (hard gate at task-40).",
  );

  for (const screen of SCREENS) {
    test(`no un-externalized copy on ${screen.name}`, async ({ testPage }) => {
      await activatePseudo(testPage, screen.url);
      // Let lazy panels settle before scanning.
      await testPage.waitForTimeout(1_000);

      const leftovers = await findUnlocalizedText(testPage, [...ALLOWED, ...(screen.allow ?? [])]);
      expect(
        leftovers,
        `Un-externalized strings on ${screen.name}:\n${leftovers.map((s) => `  - ${s}`).join("\n")}`,
      ).toEqual([]);
    });
  }
});
