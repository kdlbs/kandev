import { test } from "../../fixtures/test-base";
import { assertTasksPagePendingCleanup } from "../../helpers/task-delete-recovery";

test.use({ viewport: { width: 320, height: 740 } });

// @covers AC-TASKS-DELETE-RECOVERY-002.8
test("phone Tasks page reports pending cleanup after deleting a damaged task", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  await assertTasksPagePendingCleanup(testPage, apiClient, seedData, prCapture);
});
