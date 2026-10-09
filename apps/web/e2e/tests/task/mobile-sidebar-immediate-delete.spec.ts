import { test } from "../../fixtures/test-base";
import { checkImmediateDelete } from "./sidebar-immediate-delete-helpers";

test("delete shows a busy row, recovers on failure, and removes on success", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await checkImmediateDelete({ page: testPage, api: apiClient, seed: seedData, mobile: true });
});
