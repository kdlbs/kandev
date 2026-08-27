/**
 * Mobile-Chrome geometry coverage for opaque content rendered by
 * `PluginModalHost`. The fixture is installed through the real upload flow so
 * the test exercises the packaged `host.openModal` contract on a phone.
 */
import { expect, test } from "../../fixtures/test-base";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";

test.describe("Mobile plugin modal content", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
  });

  test("keeps long plugin content scrollable and dismissible", async ({ testPage, prCapture }) => {
    test.setTimeout(120_000);

    await installFixturePlugin(testPage);
    await testPage.goto("/");
    await testPage.reload();
    await testPage.goto("/plugins/e2e-hello");
    await expect(testPage.locator("#hello-plugin-page")).toBeVisible({ timeout: 15_000 });
    await testPage.keyboard.press("ControlOrMeta+Shift+J");

    const dialog = testPage.getByRole("dialog", { name: "Demo Modal" });
    const body = dialog.locator('[data-testid^="plugin-modal-body-"]');
    const title = dialog.locator('[data-slot="dialog-title"]');
    const close = dialog.locator('[data-slot="dialog-close"]');
    const finalAction = dialog.getByTestId("hello-long-modal-final-action");
    await expect(dialog).toBeVisible();
    await expect(body).toHaveCount(1);
    await expect(finalAction).toBeVisible();
    await dialog.evaluate(async (element) => {
      const animations = element.getAnimations({ subtree: true }).filter((animation) => {
        if (animation.playState !== "running") return false;
        const iterations = animation.effect?.getComputedTiming().iterations;
        return typeof iterations === "number" && Number.isFinite(iterations);
      });
      await Promise.all(animations.map((animation) => animation.finished.catch(() => undefined)));
    });

    const metrics = await body.evaluate((element) => {
      const node = element as HTMLElement;
      return { clientHeight: node.clientHeight, scrollHeight: node.scrollHeight };
    });
    expect(metrics.scrollHeight).toBeGreaterThan(metrics.clientHeight);

    const viewport = await testPage.evaluate(() => ({ width: innerWidth, height: innerHeight }));
    for (const [label, locator] of [
      ["dialog", dialog],
      ["title", title],
      ["close", close],
    ] as const) {
      const box = await locator.boundingBox();
      if (!box) throw new Error(`${label} has no layout box`);
      expect(box.x).toBeGreaterThanOrEqual(0);
      expect(box.y).toBeGreaterThanOrEqual(0);
      expect(box.x + box.width).toBeLessThanOrEqual(viewport.width);
      expect(box.y + box.height).toBeLessThanOrEqual(viewport.height);
    }

    await body.evaluate((element) => {
      const node = element as HTMLElement;
      node.scrollTop = node.scrollHeight;
    });
    await expect
      .poll(() => body.evaluate((element) => (element as HTMLElement).scrollTop))
      .toBeGreaterThan(0);
    await expect(finalAction).toBeInViewport();
    const finalBox = await finalAction.boundingBox();
    if (!finalBox) throw new Error("final plugin action has no layout box");
    expect(finalBox.y).toBeGreaterThanOrEqual(0);
    expect(finalBox.y + finalBox.height).toBeLessThanOrEqual(viewport.height);
    await finalAction.tap();
    await expect(finalAction).toHaveText("Plugin modal action complete");
    if (prCapture.capturing) {
      await prCapture.screenshot("long-plugin-modal", {
        caption:
          "Mobile plugin modal content stays inside one scroll body with a reachable final action.",
      });
    }

    await close.tap();
    await expect(dialog).not.toBeVisible();
    const overflow = await testPage.evaluate(
      () => document.documentElement.scrollWidth > innerWidth,
    );
    expect(overflow).toBe(false);
  });
});
