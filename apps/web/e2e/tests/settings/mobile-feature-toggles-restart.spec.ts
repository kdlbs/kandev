import { expect, test } from "../../fixtures/test-base";
import { gotoFeatureToggles, stubFeatureToggles } from "./feature-toggles-restart-helpers";

test.describe("Mobile Feature Toggles restart action", () => {
  test("keeps the supported restart action reachable", async ({ testPage }) => {
    await stubFeatureToggles(testPage, {
      supported: true,
      mode: "supervisor",
      adapter: "supervisor",
    });
    const settings = await gotoFeatureToggles(testPage);
    const restart = settings.getByRole("button", { name: "Restart", exact: true });

    await expect(restart).toBeVisible();
    await expect(settings.getByText(/terminal or service manager/)).toHaveCount(0);

    const box = await restart.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.height).toBeGreaterThanOrEqual(44);
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBe(
      await testPage.evaluate(() => document.documentElement.clientWidth),
    );
  });
});
