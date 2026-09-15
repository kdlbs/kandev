import { test, expect } from "../../fixtures/test-base";

test.describe("Settings header tabs", () => {
  test("switches Data & Logs with manual keyboard activation and keeps the panels contained", async ({
    testPage,
    prCapture,
  }) => {
    await testPage.setViewportSize({ width: 1440, height: 900 });
    await testPage.goto("/settings/system/data-storage?tab=database");

    const title = testPage.getByTestId("system-page-title");
    const tabList = testPage.getByRole("tablist", { name: "Data & Logs" });
    const databaseTab = testPage.getByRole("tab", { name: "Database", exact: true });
    const logsTab = testPage.getByRole("tab", { name: "Logs", exact: true });
    const databasePanel = testPage.getByTestId("settings-data-storage-database");
    const logsPanel = testPage.getByTestId("settings-data-storage-logs");

    await expect(title).toHaveText("Data & Logs");
    await expect(databaseTab).toHaveAttribute("aria-selected", "true");
    await expect(databasePanel).toBeVisible();
    await expect(logsPanel).toBeHidden();

    const titleBox = await title.boundingBox();
    const tabListBox = await tabList.boundingBox();
    expect(titleBox).not.toBeNull();
    expect(tabListBox).not.toBeNull();
    expect(tabListBox!.x).toBeGreaterThan(titleBox!.x + titleBox!.width);

    await logsTab.focus();
    await testPage.keyboard.press("ArrowLeft");
    await expect(databaseTab).toHaveAttribute("aria-selected", "true");
    await expect(logsTab).toHaveAttribute("aria-selected", "false");
    await testPage.keyboard.press("ArrowRight");
    await expect(logsTab).toBeFocused();
    await logsTab.press("Enter");

    await expect(logsTab).toHaveAttribute("aria-selected", "true");
    await expect(databasePanel).toBeHidden();
    await expect(logsPanel).toBeVisible();
    await expect(testPage).toHaveURL(/\/settings\/system\/data-storage\?tab=logs$/);
    await expect(testPage.getByTestId("customize-diagnostic-bundle")).toBeVisible();
    await prCapture.screenshot("desktop-data-logs-tabs", {
      caption: "Data & Logs keeps Database and Logs available from the page header.",
      fullPage: true,
    });
  });

  test("keeps a retention draft through a tab round trip and resolves legacy destinations", async ({
    testPage,
  }) => {
    await testPage.setViewportSize({ width: 1440, height: 900 });
    await testPage.goto("/settings/system/storage?tab=office-retention");

    const officeTab = testPage.getByRole("tab", { name: "Office retention", exact: true });
    const hostTab = testPage.getByRole("tab", { name: "Host", exact: true });
    await testPage.getByTestId("retention-advanced-settings").locator("summary").click();
    const batchLimit = testPage.getByTestId("retention-batch-limit");
    await expect(batchLimit).toBeVisible();
    const original = await batchLimit.inputValue();
    const updated = original === "7777" ? "8888" : "7777";

    try {
      await batchLimit.fill(updated);
      await hostTab.click();
      await expect(testPage.getByTestId("storage-settings-page")).toBeVisible();
      await officeTab.click();
      await expect(batchLimit).toHaveValue(updated);
      await expect(testPage).toHaveURL(/\/settings\/system\/storage\?tab=office-retention$/);
    } finally {
      if ((await batchLimit.inputValue()) !== original) {
        const advanced = testPage.getByTestId("retention-advanced-settings");
        if ((await advanced.getAttribute("open")) === null) {
          await advanced.locator("summary").click();
        }
        await batchLimit.fill(original);
      }
    }

    await testPage.goto("/settings/system/backups?preserve=1#legacy");
    await expect(testPage).toHaveURL(
      /\/settings\/system\/data-storage\?preserve=1&tab=database#setting-system-backups$/,
    );
    await expect(testPage.getByTestId("system-backups-card")).toBeVisible();

    await testPage.goto("/settings/system/data-storage?preserve=1#setting-system-retention");
    await expect(testPage).toHaveURL(
      /\/settings\/system\/storage\?preserve=1&tab=office-retention#setting-system-retention$/,
    );
    await expect(testPage.getByTestId("retention-policy-card")).toBeVisible();
  });

  test("opens the tab that owns a discovery target", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 1440, height: 900 });
    await testPage.goto("/settings/system/storage?tab=host#setting-system-retention");

    await expect(
      testPage.getByRole("tab", { name: "Office retention", exact: true }),
    ).toHaveAttribute("aria-selected", "true");
    await expect(testPage.getByTestId("retention-policy-card")).toBeVisible();
    await expect(testPage.getByTestId("retention-policy-card")).toHaveAttribute(
      "data-settings-target-highlight",
      "true",
      { timeout: 5_000 },
    );
  });

  test("reacts when browser history changes only the target hash", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 1440, height: 900 });
    await testPage.goto("/settings/system/storage?tab=host");
    await expect(testPage.getByRole("tab", { name: "Host", exact: true })).toHaveAttribute(
      "aria-selected",
      "true",
    );

    await testPage.evaluate(() => {
      window.location.hash = "#setting-system-retention";
    });

    await expect(
      testPage.getByRole("tab", { name: "Office retention", exact: true }),
    ).toHaveAttribute("aria-selected", "true");
    await expect(testPage.getByTestId("retention-policy-card")).toBeVisible();
  });

  test("clears a target fragment when switching away from its tab", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 1440, height: 900 });
    await testPage.goto("/settings/system/storage?tab=office-retention#setting-system-retention");

    await expect(
      testPage.getByRole("tab", { name: "Office retention", exact: true }),
    ).toHaveAttribute("aria-selected", "true");
    await testPage.getByRole("tab", { name: "Host", exact: true }).click();

    await expect(testPage).toHaveURL(/\/settings\/system\/storage\?tab=host$/);
    await expect(testPage.getByRole("tab", { name: "Host", exact: true })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });
});

// @covers AC-UI-SETTINGS-HEADER-TABS-001.3/.4/.5
for (const theme of ["light", "dark"] as const) {
  for (const surface of [
    { path: "data-storage", active: "Database", inactive: "Logs" },
    { path: "storage", active: "Host", inactive: "Office retention" },
  ]) {
    test(`distinguishes tab visual states on ${surface.path} in ${theme}`, async ({
      testPage,
    }, testInfo) => {
      await testPage.setViewportSize({ width: 1440, height: 900 });
      await testPage.addInitScript((value) => localStorage.setItem("theme", value), theme);
      await testPage.goto(`/settings/system/${surface.path}`);
      await expect(testPage.locator("html")).toHaveClass(new RegExp(`(^|\\s)${theme}(\\s|$)`));
      const active = testPage.getByRole("tab", { name: surface.active, exact: true });
      const inactive = testPage.getByRole("tab", { name: surface.inactive, exact: true });
      await expect(active).toHaveAttribute("aria-selected", "true");
      await testPage.mouse.move(0, 0);
      const inactiveBackground = await inactive.evaluate(
        (el) => getComputedStyle(el).backgroundColor,
      );
      await expect
        .poll(() => active.evaluate((el) => getComputedStyle(el).backgroundColor))
        .not.toBe(inactiveBackground);
      const selectedBackground = await active.evaluate(
        (el) => getComputedStyle(el).backgroundColor,
      );
      expect(
        await active.evaluate((element) => {
          const canvas = document.createElement("canvas");
          canvas.width = canvas.height = 1;
          const context = canvas.getContext("2d")!;
          context.fillStyle = getComputedStyle(element).backgroundColor;
          context.fillRect(0, 0, 1, 1);
          return context.getImageData(0, 0, 1, 1).data[3];
        }),
      ).toBe(255);
      const inactiveBorder = await inactive.evaluate((el) => getComputedStyle(el).borderTopColor);
      await expect
        .poll(() => active.evaluate((el) => getComputedStyle(el).borderTopColor))
        .not.toBe(inactiveBorder);
      const originalBox = await inactive.boundingBox();
      const activeBox = (await active.boundingBox())!;
      const trackBox = (await testPage.getByRole("tablist").boundingBox())!;
      expect(activeBox.height).toBeCloseTo(28, 0);
      expect(activeBox.y).toBeGreaterThanOrEqual(trackBox.y + 3);
      expect(activeBox.y + activeBox.height).toBeLessThanOrEqual(trackBox.y + trackBox.height - 3);

      await inactive.hover();
      await expect
        .poll(() => inactive.evaluate((el) => getComputedStyle(el).backgroundColor))
        .not.toBe(inactiveBackground);
      await expect(active).toHaveCSS("background-color", selectedBackground);
      await testPage.mouse.move(0, 0);
      await expect(inactive).toHaveCSS("background-color", inactiveBackground);
      const defaultShadow = await inactive.evaluate((el) => getComputedStyle(el).boxShadow);
      await active.focus();
      await active.press("ArrowRight");
      await expect(inactive).toBeFocused();
      await expect(inactive).toHaveAttribute("aria-selected", "false");
      await expect
        .poll(() => inactive.evaluate((el) => getComputedStyle(el).boxShadow))
        .not.toBe(defaultShadow);
      await inactive.press("Enter");
      await expect(inactive).toHaveAttribute("aria-selected", "true");
      await expect(inactive).toHaveCSS("background-color", selectedBackground);
      expect((await inactive.boundingBox())!.width).toBeCloseTo(originalBox!.width, 0);
      await testPage.emulateMedia({ reducedMotion: "reduce" });
      await expect(inactive).toHaveCSS("transition-property", "none");
      await testPage
        .getByRole("tablist")
        .screenshot({ path: testInfo.outputPath(`tabs-focus-${surface.path}-${theme}.png`) });
      await inactive.blur();
      await testPage.screenshot({ path: testInfo.outputPath(`tabs-${surface.path}-${theme}.png`) });
    });
  }
}
