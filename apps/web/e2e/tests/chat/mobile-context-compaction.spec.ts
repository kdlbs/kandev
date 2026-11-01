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
    const statusBox = await card.getByTestId("context-compaction-status").boundingBox();
    const tokenCountBox = await card.getByTestId("context-compaction-token-counts").boundingBox();
    expect(statusBox).toBeTruthy();
    expect(tokenCountBox).toBeTruthy();
    expect(tokenCountBox!.y).toBeGreaterThanOrEqual(statusBox!.y + statusBox!.height);

    const viewport = testPage.viewportSize();
    if (!viewport) throw new Error("mobile viewport size is unavailable");
    const documentWidth = await testPage.evaluate(() => document.documentElement.scrollWidth);
    expect(documentWidth).toBeLessThanOrEqual(viewport.width);
  });
});
