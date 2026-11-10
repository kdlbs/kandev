import { test } from "../../fixtures/test-base";
import { preserveMRLinkDraft } from "./mr-link-ownership-helpers";
for (const sameTask of [true, false]) {
  test(`phone preserves ${sameTask ? "same-task" : "different-task"} MR link opening`, async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }, info) => {
    await preserveMRLinkDraft(testPage, apiClient, seedData, {
      backend,
      info,
      mobile: true,
      sameTask,
    });
  });
}
