import { test } from "../../fixtures/test-base";
import {
  proveConsecutiveDeletes,
  proveSaveAfterFailedDelete,
  proveSaveAfterFailedLoad,
} from "../../helpers/azure-saved-views";

test("orders saved-view deletes and handles failure/retry", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await proveConsecutiveDeletes({
    page: testPage,
    apiClient,
    workspaceId: seedData.workspaceId,
    mobile: false,
  });
});

test("preserves a queued save after a failed saved-view delete", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await proveSaveAfterFailedDelete({
    page: testPage,
    apiClient,
    workspaceId: seedData.workspaceId,
    mobile: false,
  });
});

test("protects saved queries when initial loading fails and retries a save", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await proveSaveAfterFailedLoad({
    page: testPage,
    apiClient,
    workspaceId: seedData.workspaceId,
    mobile: false,
  });
});
