import { resetSeedRepositoryCheckout, test, expect } from "../../fixtures/test-base";
import { seedUntrackedFileTask } from "./diff-update-helpers";
import { markTurnChangeLegacy } from "../../helpers/turn-changes-history";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import {
  enableTurnChangedFiles,
  readTurnChangeHistory,
  reloadUntilTurnCard,
  waitForTurnChangeCount,
} from "./turn-changed-files-helpers";

test.describe("legacy turn change history", () => {
  test.describe.configure({ timeout: 120_000 });

  test("labels retained repository snapshots in the existing count slot", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    const restorePreference = await enableTurnChangedFiles(apiClient);
    try {
      resetSeedRepositoryCheckout(seedData, backend.tmpDir);
      const { session, sessionId } = await seedUntrackedFileTask(testPage, apiClient, seedData);
      await waitForTurnChangeCount(apiClient, sessionId, 1);
      const history = await readTurnChangeHistory(apiClient, sessionId);
      const changeSetId = history.change_sets?.[0]?.id;
      expect(changeSetId).toBeTruthy();
      markTurnChangeLegacy(`${backend.tmpDir}/kandev.db`, changeSetId!);

      const card = await reloadUntilTurnCard(testPage, session);
      await expect(card).toBeVisible({ timeout: 30_000 });
      await expect(card.getByText("1 repository change")).toBeVisible();
      if (prCapture.capturing) {
        await card.scrollIntoViewIfNeeded();
        await waitForFiniteAnimations(card);
        await prCapture.screenshot("turn-history-legacy-desktop", {
          caption: "Legacy row keeps repository-count wording in the existing slot",
        });
      }
    } finally {
      await restorePreference();
    }
  });
});
