import { test, expect } from "../../fixtures/test-base";

/**
 * i18n language switcher + pseudo-locale.
 *
 * The pseudo-locale accents/pads every extracted message (`Language` →
 * `Ĺàńĝũàĝē`), so it doubles as the completeness oracle for the string-
 * externalization sweep: any plain-ASCII user-facing text under `pseudo` is a
 * literal that was never routed through `t()`.
 *
 * See docs/specs/platform/requirements/i18n.md and docs/i18n.md.
 */

const APPEARANCE_URL = "/settings/preferences/appearance";

/** Latin letters carrying diacritics, as produced by scripts/generate-pseudo-locale.mjs. */
const ACCENTED = /[À-ɏ]/;

test.describe("i18n language switcher", () => {
  test("defaults to English with lang=en", async ({ testPage, prCapture }) => {
    await testPage.goto(APPEARANCE_URL);

    await expect(testPage.locator("html")).toHaveAttribute("lang", "en");
    await expect(testPage.getByLabel("Display language")).toBeVisible({ timeout: 10_000 });

    await prCapture.screenshot("language-switcher-desktop", {
      caption: "Settings > General > Appearance: the Display language selector",
    });

    await testPage.setViewportSize({ width: 390, height: 844 });
    await expect(testPage.getByLabel("Display language")).toBeVisible({ timeout: 10_000 });
    await prCapture.screenshot("language-switcher-mobile", {
      caption: "The same selector at a phone width (390x844)",
    });
  });

  test("switching to Simplified Chinese persists through reload and can restore English", async ({
    testPage,
    prCapture,
  }) => {
    await testPage.goto(APPEARANCE_URL);

    const select = testPage.getByLabel("Display language");
    await expect(select).toBeVisible({ timeout: 10_000 });
    await select.click();
    await testPage.getByRole("listbox").getByRole("option", { name: "简体中文" }).click();

    await expect(testPage.locator("html")).toHaveAttribute("lang", "zh-cn", { timeout: 10_000 });
    await expect(testPage.getByLabel("显示语言")).toBeVisible({ timeout: 10_000 });
    await expect
      .poll(async () => {
        const cookies = await testPage.context().cookies();
        return cookies.find((cookie) => cookie.name === "kandev_locale")?.value;
      })
      .toBe("zh-cn");

    await prCapture.screenshot("simplified-chinese-locale-desktop", {
      caption: "Settings > General > Appearance with Simplified Chinese active",
    });

    await testPage.reload();
    await expect(testPage.locator("html")).toHaveAttribute("lang", "zh-cn", { timeout: 10_000 });
    await expect(testPage.getByLabel("显示语言")).toBeVisible({ timeout: 10_000 });

    const selectAfter = testPage.getByLabel("显示语言");
    await selectAfter.click();
    await testPage.getByRole("listbox").getByRole("option", { name: "English" }).click();
    await expect(testPage.locator("html")).toHaveAttribute("lang", "en", { timeout: 10_000 });
    await expect(testPage.getByLabel("Display language")).toBeVisible({ timeout: 10_000 });
  });

  for (const locale of [
    {
      id: "zh-tw",
      option: "繁體中文（台灣）",
      displayLanguage: "顯示語言",
      screenshot: "traditional-chinese-taiwan-locale-desktop",
      caption: "Settings > General > Appearance with Traditional Chinese (Taiwan) active",
    },
    {
      id: "zh-hk",
      option: "繁體中文（香港）",
      displayLanguage: "顯示語言",
      screenshot: "traditional-chinese-hong-kong-locale-desktop",
      caption: "Settings > General > Appearance with Traditional Chinese (Hong Kong) active",
    },
  ] as const) {
    test(`switching to ${locale.option} persists through reload and can restore English`, async ({
      testPage,
      prCapture,
    }) => {
      await testPage.goto(APPEARANCE_URL);

      const select = testPage.getByLabel("Display language");
      await expect(select).toBeVisible({ timeout: 10_000 });
      await select.click();
      await testPage.getByRole("listbox").getByRole("option", { name: locale.option }).click();

      await expect(testPage.locator("html")).toHaveAttribute("lang", locale.id, {
        timeout: 10_000,
      });
      await expect(testPage.getByLabel(locale.displayLanguage)).toBeVisible({ timeout: 10_000 });
      await expect
        .poll(async () => {
          const cookies = await testPage.context().cookies();
          return cookies.find((cookie) => cookie.name === "kandev_locale")?.value;
        })
        .toBe(locale.id);

      await prCapture.screenshot(locale.screenshot, {
        caption: locale.caption,
      });

      await testPage.reload();
      await expect(testPage.locator("html")).toHaveAttribute("lang", locale.id, {
        timeout: 10_000,
      });
      await expect(testPage.getByLabel(locale.displayLanguage)).toBeVisible({ timeout: 10_000 });

      const selectAfter = testPage.getByLabel(locale.displayLanguage);
      await selectAfter.click();
      await testPage.getByRole("listbox").getByRole("option", { name: "English" }).click();
      await expect(testPage.locator("html")).toHaveAttribute("lang", "en", { timeout: 10_000 });
      await expect(testPage.getByLabel("Display language")).toBeVisible({ timeout: 10_000 });
    });
  }

  test("switching to pseudo re-renders accented copy and survives reload", async ({
    testPage,
    prCapture,
  }) => {
    await testPage.goto(APPEARANCE_URL);

    const select = testPage.getByLabel("Display language");
    await expect(select).toBeVisible({ timeout: 10_000 });
    await select.click();
    await testPage
      .getByRole("listbox")
      .getByRole("option", { name: /Pseudo/ })
      .click();

    // Locale activates client-side: <html lang> flips and chrome is accented.
    await expect(testPage.locator("html")).toHaveAttribute("lang", "pseudo", { timeout: 10_000 });
    await expect(testPage.locator("body")).toHaveText(ACCENTED, { timeout: 10_000 });

    await prCapture.screenshot("pseudo-locale-desktop", {
      caption:
        "Pseudo locale active: every catalog message renders accented, so any plain-ASCII text left on screen is a string that was never externalized",
    });

    // The kandev_locale cookie is the source of truth, so the Go shell serves
    // lang="pseudo" on the very next request — no flash of English.
    await testPage.reload();
    await expect(testPage.locator("html")).toHaveAttribute("lang", "pseudo", { timeout: 10_000 });

    // Switch back so the persisted cookie does not leak into later tests.
    const selectAfter = testPage.getByLabel(/Display language|Ďĩśƥĺàŷ ĺàńĝũàĝē/);
    await selectAfter.click();
    await testPage
      .getByRole("listbox")
      .getByRole("option", { name: /English|Ēńĝĺĩśĥ/ })
      .click();
    await expect(testPage.locator("html")).toHaveAttribute("lang", "en", { timeout: 10_000 });
  });
});
