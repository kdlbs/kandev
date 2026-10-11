import {
  createRetentionCapacityProfile,
  assertCapacityContinuationAfterPruning,
} from "../../helpers/delivery-retention-capacity";
import { test } from "../../fixtures/test-base";
import {
  assertRetentionRecoveryControls,
  pruneSavedDelivery,
} from "../../helpers/delivery-retention";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

test("preserves phone history and touch recovery after actual delivery cleanup", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(300_000);
  const capacity = await createRetentionCapacityProfile(apiClient, backend);
  let taskId: string | undefined;
  try {
    const { task, session, sessionId } = await pruneSavedDelivery(
      testPage,
      apiClient,
      seedData,
      backend,
      { profileId: capacity.profileId },
    );
    taskId = task.id;
    await assertCapacityContinuationAfterPruning(apiClient, backend, session, {
      taskId: task.id,
      sessionId,
      tracePath: capacity.tracePath,
    });
    await assertNoDocumentHorizontalOverflow(testPage);
    await assertRetentionRecoveryControls(testPage, apiClient, session, sessionId, {
      backend,
      touch: true,
    });
    await assertNoDocumentHorizontalOverflow(testPage);
  } finally {
    if (taskId) await apiClient.deleteTask(taskId, { discardWorktreeChanges: true });
    await apiClient.deleteAgentProfile(capacity.profileId, true);
    await backend.restart();
  }
});
