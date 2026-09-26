import fs from "node:fs";
import path from "node:path";

import { test, expect } from "../../fixtures/test-base";

/**
 * Locale catalogs are fetched on demand, not shipped in the initial download.
 *
 * `lib/i18n/index.ts` bundles `en` eagerly and glob-imports every other locale
 * lazily; `vite.config.ts` groups each one into a single `i18n-<locale>` chunk.
 * A unit test can prove the module never registered a bundle, but only a real
 * browser against a real production build can prove the BYTES never crossed the
 * wire — which is the entire point of the change.
 *
 * This spec exists because the alternative is a change that is indistinguishable
 * from a no-op: if a future refactor puts every locale back in the entry chunk,
 * every other i18n test still passes. The first assertion below is the one that
 * fails.
 *
 * It runs against the same production bundle the e2e harness always serves, so
 * the chunk names are the real ones.
 */

const APPEARANCE_URL = "/settings/general/appearance";

/**
 * The locales this spec must account for, read from the same directory listing
 * `localeChunkGroups()` in vite.config.ts reads to NAME the chunks. Deriving
 * them is the point: a hardcoded `(pt-pt|zh-cn|pseudo)` does not fail the day
 * `fr` is added — it just stops covering it, so an eagerly-bundled `fr` sails
 * past the "no non-English catalog" assertion below and an `i18n-fr-<hash>.js`
 * satisfies the "en is present" one. Both checks stay green while the thing
 * they check shrinks.
 *
 * `SUPPORTED_LOCALES` would be the more obvious source and is not reachable
 * here: importing `lib/i18n` executes `import.meta.glob` and reads
 * `__KANDEV_PSEUDO_LOCALE_BUNDLED__` at module scope, both of which exist only
 * under Vite, and Playwright loads this file in plain Node.
 *
 * A locale directory whose chunk this build did not emit — `pseudo` in a plain
 * `vite build` — needs no special case: it only ever widens a "was not
 * downloaded" assertion and an exclusion list, and no request can name it.
 */
const NON_EN_LOCALES = fs
  .readdirSync(path.resolve(__dirname, "../../../src/locales"), { withFileTypes: true })
  .filter((entry) => entry.isDirectory() && entry.name !== "en")
  .map((entry) => entry.name);

if (NON_EN_LOCALES.length === 0) {
  // Both patterns below degrade to "matches nothing" on an empty list, which is
  // a green run that checked nothing at all — the exact failure this derivation
  // exists to prevent. Refuse to load instead.
  throw new Error("no non-en locale directories under src/locales; this spec would assert nothing");
}

const LOCALE_ALTERNATION = NON_EN_LOCALES.map((locale) =>
  locale.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"),
).join("|");

const LAZY_LOCALE_CHUNK = new RegExp(`/assets/i18n-(?:${LOCALE_ALTERNATION})-[^/]+\\.js$`);
/** The eager chunk: `i18n-<hash>.js`, with no locale segment. */
const EAGER_EN_CHUNK = new RegExp(`/assets/i18n-(?!(?:${LOCALE_ALTERNATION})-)[^/]+\\.js$`);

test.describe("i18n lazy catalogs", () => {
  test("ships only en up front and fetches a locale when it is activated", async ({ testPage }) => {
    const scripts: string[] = [];
    testPage.on("request", (request) => {
      if (request.resourceType() === "script") scripts.push(new URL(request.url()).pathname);
    });

    await testPage.goto(APPEARANCE_URL);
    const select = testPage.getByLabel("Display language");
    await expect(select).toBeVisible({ timeout: 10_000 });

    // 1. An English boot downloads `en` and nothing else. This is the assertion
    //    that fails the day the catalogs go back into the entry chunk.
    expect(
      scripts.filter((path) => EAGER_EN_CHUNK.test(path)),
      "the eager en catalog chunk must be part of the initial download",
    ).not.toEqual([]);
    expect(
      scripts.filter((path) => LAZY_LOCALE_CHUNK.test(path)),
      "no non-English catalog may be downloaded before it is asked for",
    ).toEqual([]);

    // 2. Activating a locale fetches exactly that locale's chunk.
    scripts.length = 0;
    await select.click();
    await testPage.getByRole("listbox").getByRole("option", { name: "简体中文" }).click();
    await expect(testPage.getByLabel("显示语言")).toBeVisible({ timeout: 10_000 });

    const fetched = scripts.filter((path) => LAZY_LOCALE_CHUNK.test(path));
    expect(fetched, "activating zh-cn must fetch the zh-cn catalog on demand").toHaveLength(1);
    expect(fetched[0]).toContain("i18n-zh-cn-");
  });

  test("a cold boot in a non-English locale renders translated, not key names", async ({
    testPage,
  }) => {
    // The sequencing case. `main.tsx` awaits the boot locale's catalogs before
    // `createRoot().render()`, so the first committed render is already
    // translated. If that await were dropped, `returnNull: false` would paint
    // `settings:displayLanguage` — visible, wrong, and thrown by nothing.
    await testPage.goto(APPEARANCE_URL);
    await testPage.evaluate(() => {
      document.cookie = "kandev_locale=zh-cn; path=/; max-age=31536000; SameSite=Lax";
    });

    const scripts: string[] = [];
    testPage.on("request", (request) => {
      if (request.resourceType() === "script") scripts.push(new URL(request.url()).pathname);
    });
    await testPage.reload();

    await expect(testPage.locator("html")).toHaveAttribute("lang", "zh-cn", { timeout: 10_000 });
    await expect(testPage.getByLabel("显示语言")).toBeVisible({ timeout: 10_000 });

    expect(
      scripts.filter((path) => path.includes("i18n-zh-cn-")),
      "a cold zh-cn boot must fetch the zh-cn catalog",
    ).not.toEqual([]);

    // Nothing anywhere on the page rendered as a raw `namespace:key`. Scoped to
    // the catalog's own namespaces so a URL or a code identifier containing a
    // colon does not read as a finding.
    const rawKeys = await testPage.evaluate(() => {
      const pattern =
        /\b(common|settings|sidebar|kanban|task|tasks|chat|workspaces|system|account):[a-z][A-Za-z0-9_]+/g;
      return [...new Set((document.body.textContent ?? "").match(pattern) ?? [])];
    });
    expect(
      rawKeys,
      `Untranslated keys rendered on a cold zh-cn boot: ${rawKeys.join(", ")}`,
    ).toEqual([]);

    // No cookie restore here on purpose. `kandev_locale` is browser state and
    // nothing else — the switcher persists it with `document.cookie` and writes
    // nothing server-side — while the `testPage` fixture opens a fresh
    // `browser.newContext()` per test and closes it in teardown. The cookie dies
    // with the context whether this test passes or throws, so a restore step
    // would only ever be a conditional no-op that reads like a real guarantee.
  });
});
