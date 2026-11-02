import { test } from "../../fixtures/test-base";
import { preserveExternalLinkDraft } from "./external-link-ownership-helpers";

for (const sameTask of [true, false]) {
  test(`sidebar preserves ${sameTask ? "same-task" : "different-task"} link draft through earlier completion`, async ({
    testPage,
    apiClient,
    seedData,
  }, info) => {
    await preserveExternalLinkDraft(testPage, apiClient, seedData, info, {
      mobile: false,
      sameTask,
    });
  });
}
