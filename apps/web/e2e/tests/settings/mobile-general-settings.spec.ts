import { test, expect } from "../../fixtures/test-base";

test.describe("Mobile general settings", () => {
  test("keeps host sleep inhibition reachable and contained on a phone viewport", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    const initialResponse = await apiClient.rawRequest("GET", "/api/v1/system/sleep-inhibition");
    expect(initialResponse.ok).toBe(true);
    const initial = (await initialResponse.json()) as { settings: { enabled: boolean } };
    const next = !initial.settings.enabled;

    try {
      await testPage.goto("/settings/general/task-actions");
      const card = testPage.getByTestId("sleep-inhibition-settings");
      const toggle = card.getByRole("switch", { name: "Prevent idle system sleep" });
      await expect(card).toBeVisible();
      const info = card.getByRole("button", { name: "How host sleep prevention works" });
      await info.click();
      const infoDrawer = testPage.getByRole("dialog", {
        name: "How Kandev prevents host sleep",
      });
      await expect(infoDrawer).toBeVisible();
      await expect(infoDrawer).toContainText("/usr/bin/caffeinate -i -w");
      await expect(infoDrawer).toContainText("systemd-logind");
      expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBe(
        await testPage.evaluate(() => document.documentElement.clientWidth),
      );
      await prCapture.screenshot("sleep-inhibition-mobile-info", {
        caption: "Mobile host sleep prevention details in the info drawer",
      });
      const releaseCopy = infoDrawer.getByText(
        "The request does not keep the display awake or override an explicit user sleep action.",
      );
      await releaseCopy.scrollIntoViewIfNeeded();
      const releaseBox = await releaseCopy.boundingBox();
      expect(releaseBox).not.toBeNull();
      expect(releaseBox!.y + releaseBox!.height).toBeLessThanOrEqual(844);
      await testPage.keyboard.press("Escape");
      await expect(infoDrawer).not.toBeVisible();
      await toggle.click();
      await expect(toggle).toHaveAttribute("data-settings-dirty", "true");
      const controlRow = card.getByTestId("sleep-inhibition-control-row");
      const controlRowBox = await controlRow.boundingBox();
      expect(controlRowBox).not.toBeNull();
      expect(controlRowBox!.height).toBeGreaterThanOrEqual(44);

      const floatingSave = testPage.getByTestId("settings-floating-save");
      await expect(floatingSave).toBeVisible();
      await card.scrollIntoViewIfNeeded();
      const cardContent = card.locator('[data-slot="card-content"]');
      const [cardContentBox, saveBox] = await Promise.all([
        cardContent.boundingBox(),
        floatingSave.boundingBox(),
      ]);
      expect(cardContentBox).not.toBeNull();
      expect(saveBox).not.toBeNull();
      expect(cardContentBox!.y + cardContentBox!.height).toBeLessThanOrEqual(saveBox!.y + 2);
      expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBe(
        await testPage.evaluate(() => document.documentElement.clientWidth),
      );
      await testPage.waitForTimeout(1_000);
      await testPage
        .locator("[data-sonner-toast], [data-testid='toast-message']")
        .evaluateAll((toasts) => {
          for (const toast of toasts) (toast as HTMLElement).style.display = "none";
        });
      await prCapture.screenshot("sleep-inhibition-mobile-draft", {
        caption: "Mobile Task Actions sleep inhibition card above Save changes",
      });

      await floatingSave.getByRole("button", { name: "Save changes" }).click();
      await expect(floatingSave).not.toBeVisible({ timeout: 15_000 });
      const savedResponse = await apiClient.rawRequest("GET", "/api/v1/system/sleep-inhibition");
      expect(((await savedResponse.json()) as typeof initial).settings.enabled).toBe(next);
    } finally {
      await apiClient.rawRequest("PATCH", "/api/v1/system/sleep-inhibition", {
        enabled: initial.settings.enabled,
      });
    }
  });

  test("keeps the full Transcript Navigation card above the floating Save action", async ({
    testPage,
    prCapture,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.goto("/settings/general/task-actions");

    const autoScrollControl = testPage.getByRole("switch", {
      name: "Show transcript auto-scroll control",
    });
    await autoScrollControl.click();

    const [scrollContainer, transcriptCard, floatingSave] = [
      testPage.getByTestId("settings-scroll-container"),
      testPage.getByTestId("anchored-prompt-bar-card"),
      testPage.getByTestId("settings-floating-save"),
    ];
    await expect(floatingSave).toBeVisible();
    await scrollContainer.evaluate((element) => {
      element.scrollTop = element.scrollHeight;
    });

    const [cardBox, saveBox] = await Promise.all([
      transcriptCard.boundingBox(),
      floatingSave.boundingBox(),
    ]);
    expect(cardBox).not.toBeNull();
    expect(saveBox).not.toBeNull();
    expect(cardBox!.y + cardBox!.height).toBeLessThanOrEqual(saveBox!.y);
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBe(
      await testPage.evaluate(() => document.documentElement.clientWidth),
    );
    await prCapture.screenshot("transcript-navigation-save-clearance", {
      caption: "Mobile Transcript Navigation card above the floating Save action",
    });
  });

  test("keeps the floating Save reachable without covering the last control", async ({
    testPage,
    apiClient,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    const initial = await apiClient.getUserSettings();
    const initialLayout = initial.settings.changes_panel_layout === "tree" ? "tree" : "flat";
    const nextLayout = initialLayout === "tree" ? "flat" : "tree";

    try {
      await testPage.goto("/settings/general/appearance");
      const layout = testPage.getByTestId("changes-panel-layout-select");
      await layout.click();
      await testPage
        .getByRole("option", { name: nextLayout === "tree" ? "Tree" : "Flat list" })
        .click();

      const floating = testPage.getByTestId("settings-floating-save");
      const saveButton = floating.getByRole("button", { name: "Save changes" });
      await expect(saveButton).toBeVisible();
      await expect(layout).toHaveAttribute("data-settings-dirty", "true");
      await expect(testPage.getByTestId("changes-panel-layout-card")).toHaveAttribute(
        "data-settings-dirty",
        "true",
      );
      const saveBox = await saveButton.boundingBox();
      expect(saveBox).not.toBeNull();
      expect(saveBox!.height).toBeGreaterThanOrEqual(44);
      expect(saveBox!.x + saveBox!.width).toBeLessThanOrEqual(390 - 16 + 1);
      expect(saveBox!.y + saveBox!.height).toBeLessThanOrEqual(844 - 16 + 1);

      const lastControl = testPage.locator("#metrics-disk-path");
      await lastControl.scrollIntoViewIfNeeded();
      const lastControlBox = await lastControl.boundingBox();
      expect(lastControlBox).not.toBeNull();
      expect(lastControlBox!.y + lastControlBox!.height).toBeLessThanOrEqual(saveBox!.y);
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
      ).toBe(false);

      await saveButton.click();
      await expect(floating).not.toBeVisible({ timeout: 15_000 });
      await expect(layout).toHaveAttribute("data-settings-dirty", "false");
      await expect(testPage.getByTestId("changes-panel-layout-card")).toHaveAttribute(
        "data-settings-dirty",
        "false",
      );
    } finally {
      await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
        changes_panel_layout: initialLayout,
      });
    }
  });

  test("opens a dedicated General settings page from the settings index", async ({ testPage }) => {
    // `/settings` is the index on a phone: the settings tree as a page. It used
    // to be a card grid over the same list, duplicated at /settings/general.
    await testPage.goto("/settings");

    await expect(testPage.getByRole("link", { name: /Terminal/ })).toBeVisible({
      timeout: 15_000,
    });

    await testPage.getByRole("link", { name: /Terminal/ }).click();

    await expect(testPage).toHaveURL(/\/settings\/general\/terminal$/);
    await expect(testPage.getByRole("heading", { name: "Terminal", exact: true })).toBeVisible();
    await expect(testPage.getByTestId("terminal-font-select")).toBeVisible();
    await expect(testPage.getByTestId("terminal-font-size-input")).toBeVisible();
  });

  test("moves between settings pages and back home without a nav drawer", async ({ testPage }) => {
    await testPage.goto("/settings/general/terminal");

    await expect(testPage.getByRole("heading", { name: "Terminal", exact: true })).toBeVisible();
    // Settings has no hamburger: the page is the navigation, reached through the
    // breadcrumb's Settings crumb.
    await expect(testPage.getByTestId("app-nav-trigger")).toHaveCount(0);

    await testPage.getByRole("link", { name: "Settings", exact: true }).click();
    const index = testPage.getByTestId("settings-index");
    await expect(index).toBeVisible();

    await index.getByRole("link", { name: "Appearance" }).click();

    await expect(testPage).toHaveURL(/\/settings\/general\/appearance$/);
    await expect(testPage.getByRole("heading", { name: "Appearance", exact: true })).toBeVisible();

    // The topbar home crumb leaves the settings surface entirely.
    await testPage.getByTestId("topbar-phone-home").click();

    await expect(testPage).toHaveURL(/\/(?:\?.*)?$/);
    await expect(testPage.getByTestId("kanban-board")).toBeVisible();
  });
});
