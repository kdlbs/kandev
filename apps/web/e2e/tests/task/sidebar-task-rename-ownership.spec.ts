import { test } from "../../fixtures/test-base";
import { checkRenameOwnership } from "./sidebar-task-rename-ownership-helpers";

// @covers AC-UI-EDIT-FOCUS-001.7
for (const scenario of [
  { entry: "sidebar", sameTask: false },
  { entry: "sidebar", sameTask: true },
  { entry: "sidebar", sameTask: false, failure: true },
  { entry: "commands", sameTask: true },
] as const) {
  test(`${scenario.entry} preserves ${scenario.sameTask ? "reopened" : "different-task"} Rename after old ${"failure" in scenario ? "failure" : "success"}`, async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    await checkRenameOwnership({ testPage, apiClient, seedData }, testInfo, scenario);
  });
}
