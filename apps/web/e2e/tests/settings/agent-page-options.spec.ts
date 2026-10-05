import { test, expect } from "../../fixtures/test-base";
import { setStoreRole } from "../../helpers/session-store";

const SWITCH_LABEL = "Hide disabled profiles from navigation";

test.describe("Agent page options", () => {
  test("opens the compact dialog and applies the preference immediately", async ({ testPage }) => {
    // Covers AC-AGENTS-PAGE-OPTIONS-001.1 through .3 and .7.
    await testPage.goto("/settings/agents");

    const actions = testPage.getByTestId("installed-agents-actions");
    const options = actions.getByTestId("agent-options-trigger");
    await expect(options).toBeVisible({ timeout: 15_000 });
    await expect(actions.locator("button").nth(0)).toHaveText("Options");
    await expect(actions.locator("button").nth(1)).toContainText("Terminal");
    await expect(testPage.getByRole("switch", { name: SWITCH_LABEL })).toHaveCount(0);

    await options.click();
    const dialog = testPage.getByTestId("agent-options-dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog).toHaveAttribute("role", "dialog");
    await expect(dialog).toHaveAccessibleName("Agent options");
    await expect(dialog).toHaveAccessibleDescription("Changes apply immediately.");
    await expect(
      dialog.getByText("Disabled profiles remain available on this page."),
    ).toBeVisible();
    await expect(dialog.getByText("Changes apply immediately.")).toBeVisible();
    const preference = dialog.getByRole("switch", { name: SWITCH_LABEL });
    await expect(preference).toHaveAccessibleDescription(
      "Disabled profiles remain available on this page.",
    );
    await expect(preference).toHaveAttribute("aria-checked", "false");

    await preference.click();
    await expect(preference).toHaveAttribute("aria-checked", "true");
    await expect(dialog).toBeVisible();

    await dialog.getByRole("button", { name: "Done", exact: true }).click();
    await expect(dialog).toBeHidden();
    await expect(options).toBeFocused();

    await options.click();
    await expect(dialog.getByRole("switch", { name: SWITCH_LABEL })).toHaveAttribute(
      "aria-checked",
      "true",
    );
    await dialog.getByRole("button", { name: "Done", exact: true }).click();
    await testPage.reload();
    await options.click();
    await expect(dialog.getByRole("switch", { name: SWITCH_LABEL })).toHaveAttribute(
      "aria-checked",
      "true",
    );
    await expect
      .poll(
        async () => Number(await dialog.evaluate((element) => getComputedStyle(element).opacity)),
        { timeout: 5_000 },
      )
      .toBe(1);
    await testPage.screenshot({ path: test.info().outputPath("agent-options-desktop.png") });
  });

  test("supports close-button and Escape dismissal with focus return", async ({ testPage }) => {
    // Covers AC-AGENTS-PAGE-OPTIONS-001.5.
    await testPage.goto("/settings/agents");
    const options = testPage.getByTestId("agent-options-trigger");
    const dialog = testPage.getByTestId("agent-options-dialog");

    await options.click();
    await dialog.getByRole("button", { name: "Close" }).click();
    await expect(dialog).toBeHidden();
    await expect(options).toBeFocused();

    await options.click();
    await testPage.keyboard.press("Escape");
    await expect(dialog).toBeHidden();
    await expect(options).toBeFocused();
  });

  test("Enter toggles the focused preference without closing the dialog", async ({ testPage }) => {
    await testPage.goto("/settings/agents");
    const options = testPage.getByTestId("agent-options-trigger");
    const dialog = testPage.getByTestId("agent-options-dialog");

    await options.click();
    await expect(dialog).toBeVisible();
    const preference = dialog.getByRole("switch", { name: SWITCH_LABEL });
    for (let attempt = 0; attempt < 5; attempt += 1) {
      if (await preference.evaluate((element) => element === document.activeElement)) break;
      await testPage.keyboard.press("Tab");
    }

    await expect(preference).toBeFocused();
    await testPage.keyboard.press("Enter");
    await expect(preference).toHaveAttribute("aria-checked", "true");
    await expect(dialog).toBeVisible();
  });

  test("keeps Options available to a member without agent-management access", async ({
    testPage,
  }) => {
    // Covers AC-AGENTS-PAGE-OPTIONS-001.8.
    await testPage.goto("/settings/agents");
    await setStoreRole(testPage, "member");

    const options = testPage.getByTestId("agent-options-trigger");
    await expect(options).toBeVisible();
    await expect(testPage.getByTestId("new-agent-button")).toHaveCount(0);

    await options.click();
    await expect(testPage.getByTestId("agent-options-dialog")).toBeVisible();
  });

  test("keeps dialog controls touch-sized on a coarse-pointer tablet", async ({
    tabletTestPage,
  }) => {
    // Covers touch sizing when the dialog is used outside the phone breakpoint.
    await tabletTestPage.goto("/settings/agents");
    await expect.poll(() => tabletTestPage.evaluate(() => window.innerWidth)).toBe(900);
    await expect
      .poll(() => tabletTestPage.evaluate(() => matchMedia("(pointer: coarse)").matches))
      .toBe(true);

    const options = tabletTestPage.getByTestId("agent-options-trigger");
    await options.click();
    const dialog = tabletTestPage.getByTestId("agent-options-dialog");
    await expect(dialog).toBeVisible();
    await expect
      .poll(
        async () => Number(await dialog.evaluate((element) => getComputedStyle(element).opacity)),
        { timeout: 5_000 },
      )
      .toBe(1);
    const done = dialog.getByRole("button", { name: "Done", exact: true });
    const close = dialog.getByRole("button", { name: "Close", exact: true });
    const switchTarget = tabletTestPage.getByTestId("agent-options-switch-target");
    for (const control of [options, done, close, switchTarget]) {
      const box = await control.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
      if (control === switchTarget) expect(box!.width).toBeGreaterThanOrEqual(44);
    }
  });

  test("uses the drawer below 768px and preserves its open state across resize", async ({
    testPage,
  }) => {
    // Covers AC-AGENTS-PAGE-OPTIONS-001.4 and .7 at the fine-pointer boundary.
    await testPage.setViewportSize({ width: 767, height: 850 });
    await testPage.goto("/settings/agents");
    const options = testPage.getByTestId("agent-options-trigger");
    const drawer = testPage.getByTestId("agent-options-drawer");
    await options.click();
    await expect(drawer).toBeVisible();

    const preference = drawer.getByRole("switch", { name: SWITCH_LABEL });
    await preference.click();
    await expect(preference).toHaveAttribute("aria-checked", "true");
    await expect(preference).toBeFocused();

    await testPage.setViewportSize({ width: 768, height: 850 });
    const dialog = testPage.getByTestId("agent-options-dialog");
    await expect(dialog).toBeVisible();
    await expect(drawer).toHaveCount(0);
    const desktopPreference = dialog.getByRole("switch", { name: SWITCH_LABEL });
    await expect(desktopPreference).toHaveAttribute("aria-checked", "true");
    await expect(desktopPreference).toBeFocused();

    await testPage.setViewportSize({ width: 767, height: 850 });
    await expect(drawer).toBeVisible();
    await expect(dialog).toHaveCount(0);
    const mobilePreference = drawer.getByRole("switch", { name: SWITCH_LABEL });
    await expect(mobilePreference).toHaveAttribute("aria-checked", "true");
    await expect(mobilePreference).toBeFocused();

    await drawer.getByRole("button", { name: "Done", exact: true }).click();
    await expect(drawer).toHaveCount(0);
    await expect(options).toBeFocused();
  });
});
