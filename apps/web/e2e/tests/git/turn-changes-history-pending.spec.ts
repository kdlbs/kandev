import { resetSeedRepositoryCheckout, test, expect } from "../../fixtures/test-base";
import { seedUntrackedFileTask, waitForDiffText } from "./diff-update-helpers";
import { pauseTurnFileContent, resumeTurnFileContent } from "../../helpers/turn-changes-history";
import {
  enableTurnChangedFiles,
  readTurnChangeHistory,
  reloadUntilTurnCard,
  waitForTurnChangeCount,
} from "./turn-changed-files-helpers";

test.describe("paused turn content preservation", () => {
  test.describe.configure({ timeout: 120_000 });

  test("reads retained content while local export is pending without another agent turn", async ({
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
      const paused = pauseTurnFileContent(
        `${backend.tmpDir}/kandev.db`,
        changeSetId!,
        "untracked_test.txt",
      );
      const repositoryId = history.change_sets?.[0]?.repositories?.[0]?.id;
      expect(repositoryId).toBeTruthy();
      const filesResponse = await apiClient.rawRequest(
        "GET",
        `/api/v1/task-sessions/${sessionId}/turn-changes/${changeSetId}/repositories/${repositoryId}/files?offset=0&limit=100`,
      );
      expect(filesResponse.ok).toBe(true);
      const files = (await filesResponse.json()) as {
        files?: Array<{ id: string; content_availability: string }>;
      };
      expect(files.files?.find((file) => file.id === paused.fileID)?.content_availability).toBe(
        "pending",
      );

      const card = await reloadUntilTurnCard(testPage, session);
      await expect(card).toBeVisible({ timeout: 30_000 });
      const pendingFile = card.getByRole("button", { name: /untracked_test\.txt/ });
      await expect(pendingFile).toBeVisible();
      await pendingFile.click();
      const viewer = testPage.getByTestId("historical-turn-diff");
      await expect(viewer).toBeVisible();
      await waitForDiffText(testPage, "INITIAL_CONTENT");
      await session.showSessionContext();

      resumeTurnFileContent(
        `${backend.tmpDir}/kandev.db`,
        changeSetId!,
        paused.fileID,
        paused.repositoryChangeID,
        paused.links,
      );
      const reloadedCard = await reloadUntilTurnCard(testPage, session);
      await expect(reloadedCard).toBeVisible({ timeout: 30_000 });
      await reloadedCard.getByRole("button", { name: /E2E-Repo/ }).click();
      const savedFile = reloadedCard.getByRole("button", { name: /untracked_test\.txt/ });
      await expect(savedFile).toBeVisible();
      await savedFile.click();
      await waitForDiffText(testPage, "INITIAL_CONTENT");
    } finally {
      await restorePreference();
    }
  });
});
