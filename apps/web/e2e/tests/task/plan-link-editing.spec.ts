import { test } from "../../fixtures/test-base";
import { assertPlanLinkEditing } from "./plan-link-helpers";

test("link clicks keep the desktop plan editable", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  await assertPlanLinkEditing(testPage, apiClient, seedData, prCapture, false);
});
