import { test, expect } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import { exerciseSharedContextRecovery } from "./sidebar-shared-context-fixtures";
import {
  exerciseSharedPaging,
  exerciseSharedFirstResponse,
} from "./sidebar-shared-task-state-fixtures";

test.afterEach(async ({ backend }, testInfo) => {
  if (testInfo.status === testInfo.expectedStatus) return;
  await testInfo.attach("mobile-sidebar-shared-state-backend.log", {
    path: backend.logPath,
    contentType: "text/plain",
  });
});

test("phone pages complete shared state without queries and bounds archived ownership", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await exerciseSharedPaging(testPage, apiClient, seedData, true);
});
test("phone accepts safe first rows during three live invalidations", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await exerciseSharedFirstResponse(testPage, apiClient, seedData, true);
});

test("phone rejects old workspace pages and recovers shared coverage after reconnect", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
  await exerciseSharedContextRecovery(testPage, apiClient, seedData, true);
  const drawer = testPage.getByRole("dialog", { name: "Tasks", exact: true });
  await expect(drawer).toBeVisible();
  await waitForFiniteAnimations(drawer);
  await prCapture.screenshot("mobile-workspace-recovery", {
    caption: "Task picker remains usable after workspace switching and reconnect",
  });
});
