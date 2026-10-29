import { resetSeedRepositoryCheckout, test, expect } from "../../fixtures/test-base";
import { seedUntrackedFileTask } from "./diff-update-helpers";
import { addUnavailableRepositoryRoot } from "../../helpers/turn-changes-history";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import {
  enableTurnChangedFiles,
  readTurnChangeHistory,
  reloadUntilTurnCard,
  waitForTurnChangeCount,
} from "./turn-changed-files-helpers";

test.describe("mixed turn change history", () => {
  test.describe.configure({ timeout: 120_000 });

  test("keeps available counts and shows the affected repository root", async ({
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
      addUnavailableRepositoryRoot(`${backend.tmpDir}/kandev.db`, changeSetId!);

      const card = await reloadUntilTurnCard(testPage, session);
      await expect(card.getByText("1 available change")).toBeVisible();
      await expect(card.getByText("Partial")).toBeVisible();
      await expect(card.getByText(/legacy-sibling.*unavailable/)).toBeVisible();
      await expect(card).toContainText("untracked_test.txt");
      if (prCapture.capturing) {
        await card.scrollIntoViewIfNeeded();
        await waitForFiniteAnimations(card);
        await prCapture.screenshot("turn-history-partial-desktop", {
          caption: "Mixed roots keep available files and identify the affected repository",
        });
      }
    } finally {
      await restorePreference();
    }
  });
});
