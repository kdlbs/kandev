import { test, expect } from "../../fixtures/test-base";
import { openContextCompactionScenario } from "./context-compaction-helpers";

test.describe("Mobile ACP context compaction", () => {
  test.describe.configure({ retries: 0, timeout: 90_000 });

  test("keeps compaction details reachable by touch without horizontal overflow", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    test.skip(testInfo.project.name !== "mobile-chrome", "phone context compaction row");
    const { session } = await openContextCompactionScenario(
      testPage,
      apiClient,
      seedData,
      "Context compaction mobile",
    );
    const card = session.chat.getByTestId("context-compaction-message").nth(0);
    const expand = card.getByRole("button", { name: "Show compaction summary" });
    await expect(expand).toBeVisible();
    const hitBox = await expand.boundingBox();
    expect(hitBox).toBeTruthy();
    expect(hitBox!.height).toBeGreaterThanOrEqual(44);
    expect(hitBox!.x).toBeGreaterThanOrEqual(0);

    await expand.tap();
    await expect(card.getByTestId("context-compaction-summary")).toContainText(
      "Keep the delivery cursor stable.",
    );
    await expect(card.getByTestId("context-compaction-trigger")).toBeVisible();
    await expect(card.getByTestId("context-compaction-duration")).toBeVisible();
    const status = card.getByTestId("context-compaction-status");
    const tokenCounts = card.getByTestId("context-compaction-token-counts");
    await expect(status).toBeVisible();
    await expect(tokenCounts).toBeVisible();
    const metadataDoesNotOverlap = await card.evaluate((element) => {
      const status = element.querySelector('[data-testid="context-compaction-status"]');
      const tokenCounts = element.querySelector('[data-testid="context-compaction-token-counts"]');
      if (!status || !tokenCounts) return false;
      const statusBox = status.getBoundingClientRect();
      const tokenCountBox = tokenCounts.getBoundingClientRect();
      const separatedVertically =
        tokenCountBox.top >= statusBox.bottom - 1 || statusBox.top >= tokenCountBox.bottom - 1;
      const separatedHorizontally =
        tokenCountBox.left >= statusBox.right - 1 || statusBox.left >= tokenCountBox.right - 1;
      return separatedVertically || separatedHorizontally;
    });
    expect(metadataDoesNotOverlap).toBe(true);

    const viewport = testPage.viewportSize();
    if (!viewport) throw new Error("mobile viewport size is unavailable");
    const documentWidth = await testPage.evaluate(() => document.documentElement.scrollWidth);
    expect(documentWidth).toBeLessThanOrEqual(viewport.width);
  });
});
