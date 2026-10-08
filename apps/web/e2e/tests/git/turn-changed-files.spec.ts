import { test, expect } from "../../fixtures/test-base";
import {
  seedUntrackedFileTask,
  waitForDiffText,
  waitForDiffTextAbsent,
} from "./diff-update-helpers";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import {
  enableTurnChangedFiles,
  openFirstTurnChangesDiff,
  readTurnChangeHistory,
  waitForTurnChangeCount,
} from "./turn-changed-files-helpers";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";

test.describe("historical turn changed files", () => {
  test.describe.configure({ timeout: 120_000 });

  test("keeps the captured first turn after later edits and reload", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    const restorePreference = await enableTurnChangedFiles(apiClient);
    try {
      const { session, sessionId } = await seedUntrackedFileTask(testPage, apiClient, seedData);
      await waitForTurnChangeCount(apiClient, sessionId, 1);
      await expect(session.activeChat().getByTestId("turn-changed-files-card")).toHaveCount(1);

      await session.sendMessage("/e2e:untracked-file-modify");
      await expect(
        session.chat.getByText("untracked-file-modify complete", { exact: false }),
      ).toBeVisible();
      await session.waitForChatIdle();
      await waitForTurnChangeCount(apiClient, sessionId, 2);

      const git = new GitHelper(seedData.repositoryPath, makeGitEnv(backend.tmpDir));
      git.stageFile("untracked_test.txt");
      git.commit("retain later turn contents");

      await testPage.reload();
      await session.waitForLoad();
      await session.waitForChatIdle();
      await expect(session.activeChat().getByTestId("turn-changed-files-card")).toHaveCount(2);
      const history = await readTurnChangeHistory(apiClient, sessionId);
      const orderedHistory = [...(history.change_sets ?? [])].sort(
        (left, right) => left.turn_ordinal - right.turn_ordinal,
      );
      expect(orderedHistory).toHaveLength(2);
      const [firstTurn, secondTurn] = orderedHistory;
      expect(firstTurn).toBeDefined();
      expect(secondTurn).toBeDefined();

      const firstCard = session.activeChat().getByTestId("turn-changed-files-card").first();
      await expect(firstCard).toContainText("1 changed file");
      if (prCapture.capturing) {
        await firstCard.scrollIntoViewIfNeeded();
        await waitForFiniteAnimations(firstCard);
        await prCapture.screenshot("turn-changed-files-card-desktop", {
          caption: "Changed-files summary beneath its completed assistant turn",
        });
      }
      await openFirstTurnChangesDiff(session);
      const viewer = testPage.getByTestId("historical-turn-diff");
      await expect(viewer).toBeVisible();
      await expect(viewer).toContainText("untracked_test.txt");
      await waitForDiffText(testPage, "INITIAL_CONTENT");
      await waitForDiffTextAbsent(testPage, "MODIFIED_CONTENT");
      if (prCapture.capturing) {
        await waitForFiniteAnimations(viewer);
        await prCapture.screenshot("turn-changed-files-history-desktop", {
          caption: "Exact historical turn selected in the desktop Changes view",
        });
      }

      const scope = viewer.getByRole("combobox", { name: "Change scope" });
      await scope.selectOption("latest");
      await expect(viewer).toHaveAttribute("data-change-set-id", secondTurn!.id);
      await scope.selectOption(firstTurn!.id);
      await expect(viewer).toHaveAttribute("data-change-set-id", firstTurn!.id);
      await scope.selectOption("latest");
      await expect(viewer).toHaveAttribute("data-change-set-id", secondTurn!.id);
      await scope.selectOption("current");
      await expect(viewer).toHaveCount(0);
    } finally {
      await restorePreference();
    }
  });
});
