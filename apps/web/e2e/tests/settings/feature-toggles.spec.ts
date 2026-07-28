import { test } from "../../fixtures/test-base";
import { assertUnreadDividerSetting } from "./feature-toggles-helpers";

test.describe("Feature Toggles", () => {
  test("shows the enabled unread-divider setting", async ({ testPage, prCapture }, testInfo) => {
    await assertUnreadDividerSetting(testPage, prCapture, testInfo.project.name);
  });
});
