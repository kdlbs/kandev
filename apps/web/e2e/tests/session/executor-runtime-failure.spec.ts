import { expect, test } from "../../fixtures/test-base";
import { createDisabledDockerRefusal } from "./executor-runtime-failure-helpers";

test.describe("executor runtime refusal", () => {
  test("disabled Local Docker refuses instead of running on the host", async ({
    apiClient,
    backend,
    seedData,
    testPage,
  }) => {
    test.setTimeout(120_000);
    const refusal = await createDisabledDockerRefusal(apiClient, backend, seedData);
    test.skip(!refusal, "the Local Docker executor row is unavailable");

    try {
      await testPage.goto(`/t/${refusal!.taskId}`);
      await expect(testPage.locator("body")).toContainText(/unavailable|failed/i, {
        timeout: 15_000,
      });
    } finally {
      await refusal!.cleanup();
    }
  });
});
