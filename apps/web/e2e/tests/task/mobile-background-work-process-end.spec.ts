import { test } from "../../fixtures/test-base";
import { expectProcessEndClearsBackgroundWork } from "./background-work-process-end.helpers";

test.describe("background job count after the owning process ends (mobile)", () => {
  test.describe.configure({ retries: 1 });

  test("the active count clears when the agent process is replaced", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await expectProcessEndClearsBackgroundWork(testPage, apiClient, seedData);
  });
});
