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
      let residue = text;
      for (const token of allowedList) residue = residue.split(token).join(" ");
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
