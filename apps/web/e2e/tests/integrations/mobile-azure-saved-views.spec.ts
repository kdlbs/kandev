import { test } from "../../fixtures/test-base";
import {
  proveConsecutiveDeletes,
  proveSaveAfterFailedDelete,
} from "../../helpers/azure-saved-views";

test("orders phone saved-view deletes and handles failure/retry", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await proveConsecutiveDeletes({
    page: testPage,
    apiClient,
    workspaceId: seedData.workspaceId,
    mobile: true,
  });
});

test("preserves a queued phone save after a failed saved-view delete", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await proveSaveAfterFailedDelete({
    page: testPage,
    apiClient,
    workspaceId: seedData.workspaceId,
    mobile: true,
  });
});
