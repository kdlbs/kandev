import { resetSeedRepositoryCheckout, test, expect } from "../../fixtures/test-base";
import { seedUntrackedFileTask } from "./diff-update-helpers";
import { setTurnChangeOverlaps } from "../../helpers/turn-changes-history";
import {
  enableTurnChangedFiles,
  readTurnChangeHistory,
  waitForTurnChangeCount,
} from "./turn-changed-files-helpers";

test.describe("turn change overlap attribution", () => {
  test.describe.configure({ timeout: 120_000 });

  test("shows a genuine overlap and clears a stale one", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const restorePreference = await enableTurnChangedFiles(apiClient);
    try {
      resetSeedRepositoryCheckout(seedData, backend.tmpDir);
      const { session, sessionId, taskId } = await seedUntrackedFileTask(
        testPage,
        apiClient,
        seedData,
      );
      await waitForTurnChangeCount(apiClient, sessionId, 1);
      const history = await readTurnChangeHistory(apiClient, sessionId);
      const changeSetId = history.change_sets?.[0]?.id;
      expect(changeSetId).toBeTruthy();
      expect(history.change_sets?.[0]?.file_count).toBe(1);
      const dbPath = `${backend.tmpDir}/kandev.db`;
      const overlap = [
        {
          change_set_id: "other-turn",
          checkout_id: "other-checkout",
          started_at: new Date().toISOString(),
        },
      ];
      setTurnChangeOverlaps(dbPath, changeSetId!, overlap);
      await expect
        .poll(
          async () => {
            const current = await readTurnChangeHistory(apiClient, sessionId);
            return current.change_sets?.find((item) => item.id === changeSetId)?.overlap_intervals
              ?.length;
          },
          { timeout: 10_000 },
        )
        .toBe(1);
      await testPage.goto(`/t/${taskId}`);
      await session.waitForLoad();
      await session.waitForChatIdle();
      const card = session.activeChat().getByTestId("turn-changed-files-card").first();
      await expect(card).toBeVisible({ timeout: 30_000 });
      await expect(card).toContainText("Another Kandev turn overlapped this checkout.");

      setTurnChangeOverlaps(dbPath, changeSetId!, []);
      await testPage.reload();
      await session.waitForLoad();
      await session.waitForChatIdle();
      const cleared = session.activeChat().getByTestId("turn-changed-files-card").first();
      await expect(cleared).not.toContainText("Another Kandev turn overlapped this checkout.");
    } finally {
      await restorePreference();
    }
  });
});
