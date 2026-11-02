import { test } from "../../fixtures/test-base";
import { openHistoryRegression } from "./changes-history-regression-helpers";
import { expectMenu, longPressCommit, setParent } from "./root-commit-revert-helpers";

test("phone long-press menu follows known commit ancestry", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
  await openHistoryRegression(testPage, apiClient, seedData, true);
  await setParent(testPage, "");
  await longPressCommit(testPage);
  await expectMenu(testPage, false, () =>
    prCapture.screenshot("root-commit-phone", { caption: "Root commit actions on phone" }),
  );
  await setParent(testPage, "parent-sha");
  await longPressCommit(testPage);
  await expectMenu(testPage, true);
});
