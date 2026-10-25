import { expect, test } from "../../fixtures/office-fixture";

test.describe("Agent dashboard on mobile", () => {
  test("keeps all run outcome buckets reachable on a phone", async ({ testPage, officeSeed }) => {
    await testPage.goto(`/office/agents/${officeSeed.agentId}/dashboard`);

    const card = testPage.getByTestId("run-activity-card");
    await expect(card).toBeVisible({ timeout: 15_000 });
    await expect(card.getByTestId("run-activity-legend").locator(":scope > span")).toHaveCount(5);
    await expect(card.locator('[data-segment-key="skipped"]').first()).toBeAttached();
    await expect(card.locator('[data-segment-key="unclassified"]').first()).toBeAttached();

    const viewportWidth = testPage.viewportSize()?.width ?? 0;
    const cardBox = await card.boundingBox();
    expect(cardBox).not.toBeNull();
    expect(cardBox!.x + cardBox!.width).toBeLessThanOrEqual(viewportWidth);
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
  });
});
