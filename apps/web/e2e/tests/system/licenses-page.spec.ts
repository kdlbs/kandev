import { test, expect } from "../../fixtures/test-base";

test.describe("System Licenses page", () => {
  test("renders the licenses card, filters rows, and shows empty state on no match", async ({
    testPage,
  }) => {
    test.setTimeout(60_000);

    await testPage.goto("/settings/system/licenses");

    await expect(testPage.getByTestId("system-page-title")).toHaveText("Licenses");
    const card = testPage.getByTestId("system-licenses-card");
    await expect(card).toBeVisible();

    const count = testPage.getByTestId("system-licenses-count");
    await expect(count).toBeVisible();
    const fullCountText = (await count.innerText()).trim();
    expect(fullCountText.length).toBeGreaterThan(0);

    // Filter to "react" — expect at least one row and a different count value.
    const filter = testPage.getByTestId("system-licenses-filter");
    await filter.fill("react");
    await expect(testPage.locator('[data-testid="system-license-row"]').first()).toBeVisible({
      timeout: 5_000,
    });
    const filteredCountText = (await count.innerText()).trim();
    expect(filteredCountText).not.toBe(fullCountText);

    // Filter to a nonsense token → empty state.
    await filter.fill("zzz-this-package-does-not-exist");
    await expect(testPage.getByTestId("system-licenses-empty")).toBeVisible();
  });
});
