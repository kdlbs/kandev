import { test } from "../../fixtures/test-base";
import { exerciseCustomFontSecondSave } from "./terminal-custom-font-save-helpers";

test("keeps newer custom font available for a second explicit Save", async ({
  testPage,
  apiClient,
  prCapture,
}) => {
  await exerciseCustomFontSecondSave(testPage, apiClient, prCapture, false);
});
