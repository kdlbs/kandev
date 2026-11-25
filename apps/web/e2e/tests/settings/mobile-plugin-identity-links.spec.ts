import { expect, test } from "../../fixtures/test-base";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";
import { mockHumanResponsesPlugin, mockIdentityLink } from "../../helpers/plugin-identity-links";

const MIN_TOUCH_TARGET = 44;

test.describe("plugin linked chat accounts on a phone", () => {
  test("keeps every action touch-sized inside the viewport", async ({ testPage: page }) => {
    await mockHumanResponsesPlugin(page, [mockIdentityLink("link-1", "alice#0001")]);
    await installFixturePlugin(page);
    await page.goto(`/settings/plugins/${PLUGIN_ID}`);

    const card = page.getByTestId("plugin-identity-links");
    await expect(card).toBeVisible();
    const viewportWidth = page.viewportSize()?.width ?? 0;
    for (const control of [
      card.getByTestId("plugin-identity-link-code-create"),
      card.getByTestId("plugin-identity-link-revoke-link-1"),
    ]) {
      const box = await control.boundingBox();
      expect(box?.height ?? 0).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET);
      expect((box?.x ?? 0) + (box?.width ?? 0)).toBeLessThanOrEqual(viewportWidth);
    }
  });
});
