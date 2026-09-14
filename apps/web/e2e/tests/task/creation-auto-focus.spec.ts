import { test } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { verifyCreationAutoFocus } from "./creation-auto-focus-helpers";

useRegularMode();
test.setTimeout(180_000);
test("saves auto-focus and preserves listing and task context during creation", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await verifyCreationAutoFocus(testPage, apiClient, seedData.workspaceId, false);
});
