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
      await expect(testPage.locator("body")).toContainText(/unavailable|failed/i, {
        timeout: 15_000,
      });
      await assertNoDocumentHorizontalOverflow(testPage, "mobile executor runtime refusal");

      await testPage.reload();
      await expect(testPage.locator("body")).toContainText(/unavailable|failed/i, {
        timeout: 15_000,
      });
      await assertNoDocumentHorizontalOverflow(
        testPage,
        "reloaded mobile executor runtime refusal",
      );
    } finally {
      await refusal!.cleanup();
    }
  });
});
