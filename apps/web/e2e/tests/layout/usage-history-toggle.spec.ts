import { expect, test } from "../../fixtures/test-base";

test.describe("Usage history release toggle", () => {
  test("hides the Token Usage route and API while the toggle is off", async ({
    testPage,
    seedData,
    backend,
  }) => {
    const releaseFeature = await backend.useEnv({ KANDEV_FEATURES_USAGE_HISTORY: "false" });
    try {
      const apiResponse = await testPage.request.get(
        `${backend.baseUrl}/api/v1/workspaces/${seedData.workspaceId}/stats/token-usage`,
      );
      expect(apiResponse.status()).toBe(404);

      await testPage.goto(`/stats?workspaceId=${seedData.workspaceId}`);
      await expect(testPage.getByRole("tab", { name: "Token Usage" })).toHaveCount(0);

      await testPage.goto(`/stats/token-usage?workspaceId=${seedData.workspaceId}`);
      await expect(testPage).toHaveURL(/\/stats(\?|$)/);
      await expect(testPage.getByRole("tab", { name: "Token Usage" })).toHaveCount(0);
    } finally {
      await releaseFeature();
    }
  });
});
