import { test, expect } from "../../fixtures/test-base";

test.describe("Mobile general settings", () => {
  test("keeps the device chart-motion setting contained and persistent", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.addInitScript(() => {
      if (window.localStorage.getItem("kandev.settings.richOutputAnimations") === null) {
        window.localStorage.setItem("kandev.settings.richOutputAnimations", "true");
      }
    });
    await testPage.goto("/settings/preferences/appearance");

    const card = testPage.getByTestId("rich-output-motion-settings-card");
    const row = testPage.getByTestId("rich-output-motion-toggle-row");
    const toggle = card.getByRole("switch", { name: "Animate rich-output charts" });
    await card.scrollIntoViewIfNeeded();
    await expect(toggle).toBeChecked();
    const [cardBox, rowBox, toggleBox] = await Promise.all([
      card.boundingBox(),
      row.boundingBox(),
      toggle.boundingBox(),
    ]);
    expect(cardBox).not.toBeNull();
    expect(rowBox).not.toBeNull();
    expect(toggleBox).not.toBeNull();
    expect(cardBox!.x).toBeGreaterThanOrEqual(0);
    expect(cardBox!.x + cardBox!.width).toBeLessThanOrEqual(390);
    expect(rowBox!.width).toBeLessThanOrEqual(cardBox!.width);
    expect(toggleBox!.width).toBeGreaterThanOrEqual(44);
    expect(toggleBox!.height).toBeGreaterThanOrEqual(44);

    await toggle.tap();
    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" })
      .tap();
    await expect(testPage.getByTestId("settings-floating-save")).not.toBeVisible();
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBe(
      await testPage.evaluate(() => document.documentElement.clientWidth),
    );

    await testPage.reload();
    await expect(toggle).not.toBeChecked();
  });

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
      const initialControlRowBox = await controlRow.boundingBox();
      expect(initialControlRowBox).not.toBeNull();
      expect(initialControlRowBox!.height).toBeGreaterThanOrEqual(44);

      const floatingSave = testPage.getByTestId("settings-floating-save");
      await expect(floatingSave).toBeVisible();
      await card.scrollIntoViewIfNeeded();
      const [controlRowBox, saveBox] = await Promise.all([
        controlRow.boundingBox(),
        floatingSave.boundingBox(),
      ]);
      expect(controlRowBox).not.toBeNull();
      expect(saveBox).not.toBeNull();
      expect(controlRowBox!.y + controlRowBox!.height).toBeLessThanOrEqual(saveBox!.y + 2);
      expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBe(
        await testPage.evaluate(() => document.documentElement.clientWidth),
      );
      // Keeping stray toasts out of the capture. This ran unconditionally and
      // cost every mobile shard a second, even though `prCapture.screenshot` is
      // a no-op without `CAPTURE_PR_ASSETS` -- so the second bought nothing on
      // the runs that pay for it. Gated, and waiting for the toasts to go
      // rather than for a budget and then painting over whatever is left: a
      // toast that outlives this fails the capture instead of being hidden.
      if (prCapture.capturing) {
        await expect(
          testPage.locator("[data-sonner-toast], [data-testid='toast-message']"),
        ).toHaveCount(0, { timeout: 10_000 });
      }
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
    await testPage.goto("/settings/preferences/task-behavior");

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
      await testPage.goto("/settings/preferences/appearance");
      const layout = testPage.getByTestId("changes-panel-layout-select");
      await layout.click();
      await testPage
        .getByRole("option", { name: nextLayout === "tree" ? "Tree" : "Flat list" })
        .click();

      const floating = testPage.getByTestId("settings-floating-save");
      const saveButton = floating.getByRole("button", { name: "Save changes" });
      const resetButton = floating.getByRole("button", { name: "Reset" });
      const surface = floating.getByTestId("settings-floating-save-surface");
      const contentArea = testPage.getByTestId("settings-scroll-container");
      await expect(saveButton).toBeVisible();
      await expect(resetButton).toBeVisible();
      await expect(layout).toHaveAttribute("data-settings-dirty", "true");
      await expect(testPage.getByTestId("changes-panel-layout-card")).toHaveAttribute(
        "data-settings-dirty",
        "true",
      );
      const saveBox = await saveButton.boundingBox();
      const surfaceBox = await surface.boundingBox();
      expect(saveBox).not.toBeNull();
      expect(surfaceBox).not.toBeNull();
      const contentBox = await contentArea.boundingBox();
      expect(contentBox).not.toBeNull();
      expect(surfaceBox!.height).toBeLessThanOrEqual(52);
      expect(saveBox!.height).toBeGreaterThanOrEqual(44);
      expect(saveBox!.x + saveBox!.width).toBeLessThanOrEqual(390 - 16 + 1);
      expect(saveBox!.y + saveBox!.height).toBeLessThanOrEqual(844 - 16 + 1);
      expect(surfaceBox!.x).toBeGreaterThanOrEqual(0);
      expect(surfaceBox!.x + surfaceBox!.width).toBeLessThanOrEqual(390);
      expect(
        Math.abs(surfaceBox!.x + surfaceBox!.width / 2 - (contentBox!.x + contentBox!.width / 2)),
      ).toBeLessThanOrEqual(2);
      expect(
        Math.abs(surfaceBox!.y + surfaceBox!.height - (contentBox!.y + contentBox!.height - 80)),
      ).toBeLessThanOrEqual(2);
      const resetBox = await resetButton.boundingBox();
      expect(resetBox).not.toBeNull();
      expect(resetBox!.height).toBeGreaterThanOrEqual(44);

      const lastControl = testPage.locator("#metrics-disk-path");
      await lastControl.scrollIntoViewIfNeeded();
      const lastControlBox = await lastControl.boundingBox();
      expect(lastControlBox).not.toBeNull();
      expect(lastControlBox!.y + lastControlBox!.height).toBeLessThanOrEqual(saveBox!.y);
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
      ).toBe(false);

      await saveButton.tap();
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

  test("opens a dedicated Preferences page from the settings index", async ({ testPage }) => {
    // `/settings` is the index on a phone: the settings menu as a page.
    await testPage.goto("/settings");

    await expect(testPage.getByRole("link", { name: /Terminal & Editors/ })).toBeVisible({
      timeout: 15_000,
    });

    await testPage.getByRole("link", { name: /Terminal & Editors/ }).click();

    await expect(testPage).toHaveURL(/\/settings\/preferences\/terminal-editors$/);
    await expect(testPage.getByTestId("terminal-font-select")).toBeVisible();
    await expect(testPage.getByTestId("terminal-font-size-input")).toBeVisible();
  });

  test("moves between settings pages and back home without a nav drawer", async ({ testPage }) => {
    await testPage.goto("/settings/preferences/terminal-editors");

    await expect(testPage.getByTestId("terminal-font-select")).toBeVisible();
    // Settings has no hamburger: the page is the navigation, reached through the
    // breadcrumb's Settings crumb.
    await expect(testPage.getByTestId("app-nav-trigger")).toHaveCount(0);

    await testPage.getByRole("link", { name: "Settings", exact: true }).click();
    const index = testPage.getByTestId("settings-index");
    await expect(index).toBeVisible();

    await index.getByRole("link", { name: "Appearance" }).click();

    await expect(testPage).toHaveURL(/\/settings\/preferences\/appearance$/);
    await expect(
      testPage.getByRole("heading", { level: 2, name: "Appearance", exact: true }),
    ).toBeVisible();

    // The topbar home crumb leaves the settings surface entirely.
    await testPage.getByTestId("topbar-phone-home").click();

    await expect(testPage).toHaveURL(/\/(?:\?.*)?$/);
    await expect(testPage.getByTestId("kanban-board")).toBeVisible();
  });
});
