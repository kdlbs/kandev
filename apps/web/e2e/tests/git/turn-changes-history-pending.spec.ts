import { resetSeedRepositoryCheckout, test, expect } from "../../fixtures/test-base";
import { seedUntrackedFileTask, waitForDiffText } from "./diff-update-helpers";
import {
  pauseTurnFileContent,
  resumeTurnFileContent,
  type PausedTurnFileContent,
} from "../../helpers/turn-changes-history";
import { waitForHttp } from "../../helpers/causal-waits";
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
    let changeSetIdForCleanup: string | undefined;
    let pausedContentForCleanup: PausedTurnFileContent | undefined;
    let contentResumed = false;
    try {
      resetSeedRepositoryCheckout(seedData, backend.tmpDir);
      const { session, sessionId } = await seedUntrackedFileTask(testPage, apiClient, seedData);
      await waitForTurnChangeCount(apiClient, sessionId, 1);
      const history = await readTurnChangeHistory(apiClient, sessionId);
      const changeSetId = history.change_sets?.[0]?.id;
      changeSetIdForCleanup = changeSetId;
      expect(changeSetId).toBeTruthy();
      const paused = pauseTurnFileContent(
        `${backend.tmpDir}/kandev.db`,
        changeSetId!,
        "untracked_test.txt",
      );
      pausedContentForCleanup = paused;

      const card = await reloadUntilTurnCard(testPage, session);
      await expect(card).toBeVisible({ timeout: 30_000 });
      const pendingFile = card.getByRole("button", { name: /untracked_test\.txt/ });
      await expect(pendingFile).toBeVisible();
      const contentPath = new RegExp(
        `/api/v1/task-sessions/${sessionId}/turn-changes/${changeSetId!}/files/${paused.fileID}/content$`,
      );
      const waitForContentRead = () =>
        waitForHttp(testPage, "GET", contentPath, {
          predicate: (response) =>
            new URL(response.url()).searchParams.get("variant") === "canonical_patch",
        });
      const pendingContentRead = waitForContentRead();
      await pendingFile.click();
      expect((await pendingContentRead).status()).toBe(404);
      const viewer = testPage.getByTestId("historical-turn-diff");
      await expect(viewer).toBeVisible();
      await expect(viewer.getByText("Loading changed files...")).toBeVisible();
      await session.showSessionContext();

      resumeTurnFileContent(`${backend.tmpDir}/kandev.db`, changeSetId!, paused);
      contentResumed = true;
      const reloadedCard = await reloadUntilTurnCard(testPage, session);
      await expect(reloadedCard).toBeVisible({ timeout: 30_000 });
      await reloadedCard.getByRole("button", { name: /E2E-Repo/ }).click();
      const savedFile = reloadedCard.getByRole("button", { name: /untracked_test\.txt/ });
      await expect(savedFile).toBeVisible();
      const savedContentRead = waitForContentRead();
      await savedFile.click();
      expect((await savedContentRead).status()).toBe(200);
      await waitForDiffText(testPage, "INITIAL_CONTENT");
    } finally {
      try {
        if (changeSetIdForCleanup && pausedContentForCleanup && !contentResumed) {
          resumeTurnFileContent(
            `${backend.tmpDir}/kandev.db`,
            changeSetIdForCleanup,
            pausedContentForCleanup,
          );
        }
      } finally {
        await restorePreference();
      }
    }
  });
});
