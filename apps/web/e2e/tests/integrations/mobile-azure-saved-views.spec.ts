import { test } from "../../fixtures/test-base";
import {
  proveConsecutiveDeletes,
  proveSaveAfterFailedDelete,
  proveSaveAfterFailedLoad,
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

test("protects phone saved queries when initial loading fails and retries a save", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await proveSaveAfterFailedLoad({
    page: testPage,
    apiClient,
    workspaceId: seedData.workspaceId,
    mobile: true,
  });
});
