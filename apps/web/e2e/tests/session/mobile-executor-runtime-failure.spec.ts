import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { createDisabledDockerRefusal } from "./executor-runtime-failure-helpers";

test.describe("executor runtime refusal on mobile", () => {
  test("disabled Local Docker refuses without horizontal overflow", async ({
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
      await assertNoDocumentHorizontalOverflow(testPage, "mobile executor runtime refusal");

      await testPage.reload();
      await expect(sessionError).toBeVisible({ timeout: 15_000 });
      await expect(sessionError).toContainText(
        /executor runtime .*unavailable|executor_runtime_unavailable/i,
      );
      await assertNoDocumentHorizontalOverflow(
        testPage,
        "reloaded mobile executor runtime refusal",
      );
    } finally {
      await refusal!.cleanup();
    }
  });
});
