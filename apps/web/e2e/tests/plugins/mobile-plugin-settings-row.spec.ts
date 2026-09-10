/**
 * E2E: a plugin row opens its settings page from a phone.
 *
 * The row used to carry its link on the plugin name alone, styled like the
 * headings around it and underlined only on hover, so a touch device got no
 * affordance at all. The whole card is the target now, with a chevron. This is
 * the phone half of that: `e2e/playwright.config.ts` routes only
 * `mobile-*.spec.ts` at the Pixel 5 project, so the desktop coverage in
 * plugins.spec.ts cannot catch phone wrapping, stacking, or hit-target
 * regressions in the row.
 *
 * One test rather than several: installing the fixture plugin is the expensive
 * part of the setup and every assertion here is about the same rendered row.
 */
import { expect, test } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { PLUGIN_ID, installFixturePlugin } from "../../helpers/plugin-fixture";

test("mobile plugin row: whole card opens settings, controls still act", async ({
  testPage,
  apiClient,
}) => {
  test.setTimeout(180_000);

  const baselineSettings = await apiClient.getUserSettings();
  const baselineShortcuts = baselineSettings.settings.keyboard_shortcuts ?? {};

  try {
    await installFixturePlugin(testPage);
    const pluginRow = testPage.getByTestId(`plugin-row-${PLUGIN_ID}`);
    await expect(pluginRow).toBeVisible({ timeout: 30_000 });

    // The fixture declares a required api_token nobody has filled in, so the row
    // advertises the page it wants opened. It has to survive the phone's badge
    // wrapping, not just fit on a desktop title row.
    await expect(pluginRow.getByTestId(`plugin-setup-required-${PLUGIN_ID}`)).toBeVisible({
      timeout: 15_000,
    });

    // The overlay link is the whole affordance on touch, where nothing hovers,
    // so it has to actually cover the card. It is inset by the row's 1px border,
    // hence the 2px slack; a `space-y-*` margin landing on it once cost the
    // bottom strip of the card.
    const linkBox = await pluginRow.getByTestId(`plugin-row-link-${PLUGIN_ID}`).boundingBox();
    const rowBox = await pluginRow.boundingBox();
    if (!linkBox || !rowBox) throw new Error("plugin row or its overlay link has no layout box");
    expect(linkBox.width).toBeGreaterThanOrEqual(rowBox.width - 2);
    expect(linkBox.height).toBeGreaterThanOrEqual(rowBox.height - 2);

    // Uninstall keeps its confirmation inside the row on a coarse pointer.
    // Cancel first so the same fixture can cover the existing disable action
    // and the detail-surface confirmation below.
    const rowUninstall = pluginRow.getByRole("button", { name: "Uninstall" });
    expect((await rowUninstall.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await rowUninstall.tap();
    const rowConfirmation = pluginRow.getByTestId("plugin-uninstall-inline-confirmation");
    await expect(rowConfirmation).toBeVisible();
    await expect(testPage.locator('[data-slot="dialog-overlay"]')).toHaveCount(0);
    expect(
      (await rowConfirmation.getByTestId("plugin-uninstall-confirm").boundingBox())?.height,
    ).toBeGreaterThanOrEqual(44);
    await rowConfirmation.getByRole("button", { name: "Cancel" }).tap();
    await expect(rowConfirmation).toHaveCount(0);

    // Every control sits above the overlay. If one slipped below it the tap would
    // navigate instead of acting, and on a phone there is no hover state to
    // reveal which is which.
    const disable = pluginRow.getByRole("button", { name: "Disable" });
    expect((await disable.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await disable.tap();
    await expect(pluginRow.getByText("Disabled", { exact: true })).toBeVisible({ timeout: 15_000 });
    await expect(testPage).toHaveURL(/\/settings\/plugins$/);

    // Tapping the card body rather than the name is the whole point of the
    // change; keep clear of the row's own controls.
    await pluginRow.tap({ position: { x: 12, y: 12 } });
    await expect(testPage).toHaveURL(new RegExp(`/settings/plugins/${PLUGIN_ID}$`));
    const detail = testPage.getByTestId(`plugin-detail-${PLUGIN_ID}`);
    await expect(detail).toBeVisible();

    // Disabled plugins retain editable declarations, but their handlers only run when active.
    const shortcutCard = detail.getByTestId("plugin-shortcuts-card");
    await expect(shortcutCard).toBeVisible();
    const shortcutRecorder = shortcutCard.getByTestId(
      `shortcut-recorder-plugin:${PLUGIN_ID}:open-demo`,
    );
    const recorderBox = await shortcutRecorder.boundingBox();
    expect(recorderBox?.height).toBeGreaterThanOrEqual(44);
    const settingsPanel = testPage.locator('[data-testid="settings-scroll-container"]:visible');
    const scrollState = await settingsPanel.evaluate((element) => ({
      overflowY: getComputedStyle(element).overflowY,
      scrollHeight: element.scrollHeight,
      clientHeight: element.clientHeight,
    }));
    expect(scrollState.overflowY).toMatch(/auto|scroll/);
    expect(scrollState.scrollHeight).toBeGreaterThanOrEqual(scrollState.clientHeight);
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(
      await testPage.evaluate(() => document.documentElement.clientWidth),
    );

    await shortcutRecorder.tap();
    await testPage.keyboard.press("Control+Alt+p");
    await expect(shortcutRecorder).toHaveText("Ctrl+Alt+P");
    const reset = shortcutCard.getByRole("button", { name: "Reset to default" });
    expect((await reset.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    const settingsSaved = waitForHttp(testPage, "PATCH", /^\/api\/v1\/user\/settings$/, {
      predicate: (response) => response.ok(),
    });
    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", {
        name: "Save changes",
      })
      .tap();
    await settingsSaved;
    await testPage.reload();
    await expect(
      testPage
        .getByTestId("plugin-shortcuts-card")
        .getByTestId(`shortcut-recorder-plugin:${PLUGIN_ID}:open-demo`),
    ).toHaveText("Ctrl+Alt+P");
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(
      await testPage.evaluate(() => document.documentElement.clientWidth),
    );

    // The detail danger zone uses the same phone-native inline confirmation and
    // keeps both actions at the 44px touch target minimum.
    const detailUninstall = detail.getByRole("button", { name: "Uninstall" });
    expect((await detailUninstall.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await detailUninstall.tap();
    const detailConfirmation = testPage.getByTestId("plugin-uninstall-inline-confirmation");
    await expect(detailConfirmation).toBeVisible();
    await expect(testPage.locator('[data-slot="dialog-overlay"]')).toHaveCount(0);
    expect(
      (await detailConfirmation.getByTestId("plugin-uninstall-confirm").boundingBox())?.height,
    ).toBeGreaterThanOrEqual(44);
    await detailConfirmation.getByTestId("plugin-uninstall-confirm").tap();
    await expect(testPage).toHaveURL(/\/settings\/plugins$/);
    await expect(testPage.getByTestId(`plugin-row-${PLUGIN_ID}`)).toHaveCount(0);
  } finally {
    await apiClient.saveUserSettings({ keyboard_shortcuts: baselineShortcuts });
  }
});
