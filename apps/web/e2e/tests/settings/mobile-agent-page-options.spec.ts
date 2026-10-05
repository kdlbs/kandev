import { test, expect } from "../../fixtures/test-base";

const SWITCH_LABEL = "Hide disabled profiles from navigation";

test.describe("Mobile agent page options", () => {
  test("opens the inset drawer, keeps disabled profiles listed, and saves the preference", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Covers AC-AGENTS-PAGE-OPTIONS-001.3 through .8.
    await apiClient.updateAgentProfile(seedData.agentProfileId, { enabled: false });
    try {
      await testPage.goto("/settings/agents");
      const options = testPage.getByTestId("agent-options-trigger");
      await expect(options).toBeVisible({ timeout: 15_000 });
      await expect
        .poll(
          async () => {
            const box = await options.boundingBox();
            return box ? box.height : null;
          },
          { timeout: 10_000 },
        )
        .toBeGreaterThanOrEqual(44);
      const optionsBox = await options.boundingBox();
      expect(optionsBox).not.toBeNull();
      expect(optionsBox!.height).toBe(44);

      await options.tap();
      const drawer = testPage.getByTestId("agent-options-drawer");
      const surface = testPage.getByTestId("agent-options-mobile-card");
      await expect(drawer).toBeVisible();
      await expect(drawer.getByRole("heading", { name: "Agent options" })).toBeVisible();
      await expect(drawer).toHaveAccessibleDescription("Changes apply immediately.");
      const viewport = await testPage.evaluate(() => ({
        width: window.innerWidth,
        height: window.innerHeight,
      }));
      await expect
        .poll(
          async () => {
            const box = await drawer.boundingBox();
            return box ? box.y + box.height <= viewport.height : false;
          },
          { timeout: 10_000 },
        )
        .toBe(true);
      const [drawerBox, surfaceBox] = await Promise.all([
        drawer.boundingBox(),
        surface.boundingBox(),
      ]);
      expect(drawerBox).not.toBeNull();
      expect(surfaceBox).not.toBeNull();
      expect(drawerBox!.x).toBeGreaterThanOrEqual(0);
      expect(drawerBox!.y).toBeGreaterThanOrEqual(0);
      expect(drawerBox!.x + drawerBox!.width).toBeLessThanOrEqual(viewport.width);
      expect(drawerBox!.y + drawerBox!.height).toBeLessThanOrEqual(viewport.height);
      expect(drawerBox!.height).toBeLessThan(viewport.height * 0.7);
      expect(surfaceBox!.x).toBeGreaterThanOrEqual(drawerBox!.x);
      expect(surfaceBox!.x + surfaceBox!.width).toBeLessThanOrEqual(
        drawerBox!.x + drawerBox!.width,
      );

      const scrollRegion = drawer.getByTestId("agent-options-mobile-scroll");
      await expect(scrollRegion).toBeVisible();
      expect(
        await drawer.locator("*").evaluateAll(
          (elements) =>
            elements.filter((element) => {
              const overflowY = getComputedStyle(element).overflowY;
              return overflowY === "auto" || overflowY === "scroll";
            }).length,
        ),
      ).toBe(1);
      const footer = drawer.getByTestId("agent-options-mobile-footer");
      const footerBottomPadding = Number.parseFloat(
        await footer.evaluate((element) => getComputedStyle(element).paddingBottom),
      );
      expect(footerBottomPadding).toBeGreaterThanOrEqual(16);

      const switchTarget = drawer.getByTestId("agent-options-switch-target");
      const targetBox = await switchTarget.boundingBox();
      expect(targetBox).not.toBeNull();
      expect(targetBox!.width).toBe(44);
      expect(targetBox!.height).toBe(44);
      const preference = drawer.getByRole("switch", { name: SWITCH_LABEL });
      await expect(preference).toHaveAccessibleDescription(
        "Disabled profiles remain available on this page.",
      );
      await expect(preference).toHaveAttribute("aria-checked", "false");
      await switchTarget.tap();
      await expect(preference).toHaveAttribute("aria-checked", "true");
      await expect(drawer).toBeVisible();

      const rows = testPage.getByTestId("agent-profile-row");
      const disabledProfileRow = rows.filter({ hasText: "Disabled" });
      await expect(disabledProfileRow).toBeVisible();
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      ).toBe(true);

      const done = drawer.getByRole("button", { name: "Done", exact: true });
      const doneBox = await done.boundingBox();
      expect(doneBox).not.toBeNull();
      expect(doneBox!.height).toBe(48);
      await test.info().attach("agent-options-mobile-measurements.txt", {
        body: [
          `Options button: ${optionsBox!.height}px high`,
          `Switch target: ${targetBox!.width}x${targetBox!.height}px`,
          `Done button: ${doneBox!.height}px high`,
          `Drawer footer bottom padding: ${footerBottomPadding}px`,
        ].join("\n"),
      });
      await testPage.screenshot({ path: test.info().outputPath("agent-options-mobile.png") });
      await done.tap();
      await expect(drawer).toBeHidden();
      await expect(options).toBeFocused();

      await options.tap();
      await expect(drawer.getByRole("switch", { name: SWITCH_LABEL })).toHaveAttribute(
        "aria-checked",
        "true",
      );
      await drawer.getByRole("button", { name: "Done", exact: true }).tap();
      await testPage.reload();
      await expect(options).toBeVisible();
      await options.tap();
      await expect(drawer.getByRole("switch", { name: SWITCH_LABEL })).toHaveAttribute(
        "aria-checked",
        "true",
      );
    } finally {
      await apiClient.updateAgentProfile(seedData.agentProfileId, { enabled: true });
    }
  });
});
