import { expect, test } from "../../fixtures/test-base";

const FINAL_SOURCE_ID = "source-31";

function longMarketplaceCatalog() {
  return {
    plugins: [],
    sources: Array.from({ length: 32 }, (_, index) => ({
      id: `source-${index}`,
      name: `Registry ${index}`,
      url: `https://registry-${index}.example/index.json`,
      enabled: true,
      builtin: index === 0,
      healthy: true,
      created_at: "2026-01-01T00:00:00.000Z",
    })),
  };
}

async function waitForDialog(page: import("@playwright/test").Page) {
  await page.goto("/settings/plugins");
  await page.getByTestId("plugins-tab-browse").click();
  await expect(page.getByTestId("marketplace-manage-sources")).toBeVisible();
  await page.getByTestId("marketplace-manage-sources").click();

  const dialog = page.getByTestId("marketplace-sources-dialog");
  await expect(dialog).toBeVisible();
  return dialog;
}

test.describe("Marketplace source dialog", () => {
  test("keeps source rows scrolling while title, close, and add form stay reachable", async ({
    testPage,
  }) => {
    test.setTimeout(60_000);
    await testPage.route("**/api/plugins/marketplace*", async (route) => {
      if (route.request().method() !== "GET") {
        await route.fallback();
        return;
      }
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(longMarketplaceCatalog()),
      });
    });

    const dialog = await waitForDialog(testPage);
    const list = dialog.getByTestId("marketplace-sources-list");
    const addForm = dialog.getByTestId("marketplace-add-source-form");
    const title = dialog.locator('[data-slot="dialog-title"]');
    const close = dialog.locator('[data-slot="dialog-close"]');
    const finalSource = list.getByTestId(`marketplace-source-${FINAL_SOURCE_ID}`);

    const scrollable = await list.evaluate((element) => ({
      scrollHeight: element.scrollHeight,
      clientHeight: element.clientHeight,
    }));
    expect(scrollable.scrollHeight).toBeGreaterThan(scrollable.clientHeight);

    const viewportHeight = await testPage.evaluate(() => window.innerHeight);
    const viewportWidth = await testPage.evaluate(() => window.innerWidth);
    const dialogBox = await dialog.boundingBox();
    const titleBox = await title.boundingBox();
    const closeBox = await close.boundingBox();
    const addFormBox = await addForm.boundingBox();
    expect(dialogBox).not.toBeNull();
    expect(titleBox).not.toBeNull();
    expect(closeBox).not.toBeNull();
    expect(addFormBox).not.toBeNull();
    expect(dialogBox!.x).toBeGreaterThanOrEqual(0);
    expect(dialogBox!.x + dialogBox!.width).toBeLessThanOrEqual(viewportWidth);
    expect(dialogBox!.y).toBeGreaterThanOrEqual(0);
    expect(dialogBox!.y + dialogBox!.height).toBeLessThanOrEqual(viewportHeight);
    expect(titleBox!.y).toBeGreaterThanOrEqual(dialogBox!.y);
    expect(titleBox!.y + titleBox!.height).toBeLessThanOrEqual(dialogBox!.y + dialogBox!.height);
    expect(closeBox!.y).toBeGreaterThanOrEqual(dialogBox!.y);
    expect(closeBox!.y + closeBox!.height).toBeLessThanOrEqual(dialogBox!.y + dialogBox!.height);
    expect(addFormBox!.y).toBeGreaterThanOrEqual(dialogBox!.y);
    expect(addFormBox!.y + addFormBox!.height).toBeLessThanOrEqual(
      dialogBox!.y + dialogBox!.height,
    );
    await expect(dialog.getByTestId("marketplace-add-source-url")).toBeVisible();
    await expect(dialog.getByTestId("marketplace-add-source-submit")).toBeDisabled();

    await list.evaluate((element) => {
      element.scrollTop = element.scrollHeight;
    });
    await expect.poll(() => list.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
    await expect(finalSource).toBeVisible();

    const finalSourceBox = await finalSource.boundingBox();
    const finalRemove = finalSource.getByRole("button", { name: /Remove/i });
    const finalRemoveBox = await finalRemove.boundingBox();
    const addFormAfterBox = await addForm.boundingBox();
    expect(finalSourceBox).not.toBeNull();
    expect(finalRemoveBox).not.toBeNull();
    expect(addFormAfterBox).not.toBeNull();
    expect(finalSourceBox!.y).toBeGreaterThanOrEqual(dialogBox!.y);
    expect(finalSourceBox!.y + finalSourceBox!.height).toBeLessThanOrEqual(
      dialogBox!.y + dialogBox!.height,
    );
    expect(finalRemoveBox!.y).toBeGreaterThanOrEqual(finalSourceBox!.y);
    expect(finalRemoveBox!.y + finalRemoveBox!.height).toBeLessThanOrEqual(
      finalSourceBox!.y + finalSourceBox!.height,
    );
    expect(addFormAfterBox!.y).toBeGreaterThanOrEqual(dialogBox!.y);
    expect(addFormAfterBox!.y + addFormAfterBox!.height).toBeLessThanOrEqual(
      dialogBox!.y + dialogBox!.height,
    );
    await expect(dialog.getByTestId("marketplace-add-source-url")).toBeVisible();
    await expect(dialog.getByTestId("marketplace-add-source-submit")).toBeDisabled();
  });
});
