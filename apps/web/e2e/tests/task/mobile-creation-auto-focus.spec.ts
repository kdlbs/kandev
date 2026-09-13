import { test } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { verifyCreationAutoFocus } from "./creation-auto-focus-helpers";

useRegularMode();
test.setTimeout(180_000);
test("keeps mobile creation in context with a saved touch-accessible preference", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await verifyCreationAutoFocus(testPage, apiClient, seedData.workspaceId, true);
});
