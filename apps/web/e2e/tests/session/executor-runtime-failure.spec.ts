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
      const sessionError = testPage.getByTestId("ensure-session-error-banner");
      await expect(sessionError).toBeVisible({ timeout: 15_000 });
      await expect(sessionError).toContainText(
        /executor runtime .*unavailable|executor_runtime_unavailable/i,
      );
    } finally {
      await refusal!.cleanup();
    }
  });
});
