import { test, expect } from "../../fixtures/test-base";

// Below `md` there is no sidebar, so `/settings` is the settings list itself —
// the same tree the nav sheet shows, rendered as a real route rather than an
// overlay the app has to open for you.
test.describe("Settings index on a phone", () => {
  test("renders the settings tree as the page and navigates from it", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.goto("/settings");

    const index = testPage.getByTestId("settings-index");
    await expect(index).toBeVisible();
    // Stays on /settings: no desktop-style handoff, and nothing to go Back past.
    await expect(testPage).toHaveURL(/\/settings$/);

    await index.getByRole("link", { name: /Terminal/ }).click();

    await expect(testPage).toHaveURL(/\/settings\/general\/terminal$/);
    await expect(testPage.getByTestId("terminal-font-select")).toBeVisible();
  });

  test("offers no nav drawer anywhere in settings", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });

    for (const path of ["/settings", "/settings/general/terminal", "/settings/prompts"]) {
      await testPage.goto(path);
      await expect(testPage.getByTestId("settings-scroll-container")).toBeVisible();
      // A sheet here would offer the list the index page already is.
      await expect(testPage.getByTestId("app-nav-trigger")).toHaveCount(0);
    }
  });

  test("keeps the search field in thumb reach at the bottom of the index", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.goto("/settings");

    // The desktop sidebar's tree is in the DOM too (hidden below md) and carries
    // the same testid, so scope to the page's copy.
    const search = testPage.getByTestId("settings-index").getByTestId("settings-search");
    await expect(search).toBeVisible();

    const [box, viewport] = [
      await search.boundingBox(),
      await testPage.evaluate(() => ({ w: innerWidth, h: innerHeight })),
    ];
    expect(box).not.toBeNull();
    expect(box!.y).toBeGreaterThan(viewport.h / 2);
    expect(box!.y + box!.height).toBeLessThanOrEqual(viewport.h);

    // And clear of the config-chat button, which shares that corner.
    const chat = testPage.getByRole("button", { name: "Configuration Chat" });
    const chatBox = await chat.boundingBox();
    expect(chatBox).not.toBeNull();
    expect(box!.x + box!.width).toBeLessThanOrEqual(chatBox!.x);

    // Still filters the list it floats over.
    await search.getByRole("searchbox", { name: "Search settings" }).fill("terminal font size");
    await expect(
      testPage.getByTestId("settings-index").getByTestId("settings-search-results"),
    ).toBeVisible();
  });
});
