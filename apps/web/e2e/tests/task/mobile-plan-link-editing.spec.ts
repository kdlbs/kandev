import { test } from "../../fixtures/test-base";
import { assertPlanLinkEditing } from "./plan-link-helpers";

test("link taps keep the phone plan editable", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  await assertPlanLinkEditing(testPage, apiClient, seedData, prCapture, true);
});
