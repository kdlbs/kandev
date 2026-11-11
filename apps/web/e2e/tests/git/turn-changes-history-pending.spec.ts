import { resetSeedRepositoryCheckout, test, expect } from "../../fixtures/test-base";
import { seedUntrackedFileTask, waitForDiffText } from "./diff-update-helpers";
import {
  pauseTurnFileContent,
  resumeTurnFileContent,
  waitForTurnFileContentSettled,
} from "../../helpers/turn-changes-history";
import {
  enableTurnChangedFiles,
  readTurnChangeHistory,
  reloadUntilTurnCard,
  waitForTurnChangeCount,
} from "./turn-changed-files-helpers";

test.describe("paused turn content preservation", () => {
  test.describe.configure({ timeout: 120_000 });

  test("shows the existing loading state, then opens saved content without another agent turn", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const restorePreference = await enableTurnChangedFiles(apiClient);
    try {
      resetSeedRepositoryCheckout(seedData, backend.tmpDir);
      const { session, sessionId } = await seedUntrackedFileTask(testPage, apiClient, seedData);
      await waitForTurnChangeCount(apiClient, sessionId, 1);
      const history = await readTurnChangeHistory(apiClient, sessionId);
      const changeSetId = history.change_sets?.[0]?.id;
      expect(changeSetId).toBeTruthy();
      await waitForTurnFileContentSettled(
        `${backend.tmpDir}/kandev.db`,
        changeSetId!,
        "untracked_test.txt",
      );
      const paused = pauseTurnFileContent(
        `${backend.tmpDir}/kandev.db`,
        changeSetId!,
        "untracked_test.txt",
      );

      const pausedContent = await apiClient.rawRequest(
        "GET",
        `/api/v1/task-sessions/${sessionId}/turn-changes/${changeSetId}/files/${paused.fileID}/content?variant=canonical_patch`,
      );
      expect(pausedContent.status).toBe(404);

      const card = await reloadUntilTurnCard(testPage, session);
      await expect(card).toBeVisible({ timeout: 30_000 });
      const pendingFile = card.getByRole("button", { name: /untracked_test\.txt/ });
      await expect(pendingFile).toBeVisible();
      await pendingFile.click();
      const viewer = testPage.getByTestId("historical-turn-diff");
      await expect(viewer).toBeVisible();
      await expect(viewer.getByText("Loading changed files...")).toBeVisible();
      await session.showSessionContext();

      resumeTurnFileContent(`${backend.tmpDir}/kandev.db`, changeSetId!, paused);
      const reloadedCard = await reloadUntilTurnCard(testPage, session);
      await expect(reloadedCard).toBeVisible({ timeout: 30_000 });
      await reloadedCard.getByRole("button", { name: /E2E-Repo/ }).click();
      const savedFile = reloadedCard.getByRole("button", { name: /untracked_test\.txt/ });
      await expect(savedFile).toBeVisible();
      await savedFile.click();
      await waitForDiffText(testPage, "INITIAL_CONTENT");
      await waitForTurnChangeCount(apiClient, sessionId, 1);
    } finally {
      await restorePreference();
    }
  });
});
