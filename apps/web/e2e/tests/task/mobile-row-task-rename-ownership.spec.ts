import { test } from "../../fixtures/test-base";
import { checkRenameOwnership } from "./sidebar-task-rename-ownership-helpers";

// @covers AC-UI-EDIT-FOCUS-001.7
for (const tablet of [false, true]) {
  test(`task command Rename preserves a reopened draft on ${tablet ? "tablet" : "phone"}`, async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }, testInfo) => {
    await checkRenameOwnership({ testPage, apiClient, seedData, backend }, testInfo, {
      entry: "commands",
      sameTask: true,
      touch: true,
      tablet,
    });
  });
}
