import { test, expect } from "../../fixtures/test-base";
import { waitForDiffText, waitForDiffTextAbsent } from "./diff-update-helpers";
import { seedUntrackedFileTask } from "./diff-update-helpers";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import {
  enableTurnChangedFiles,
  openFirstTurnChangesDiff,
  readTurnChangeHistory,
  waitForTurnChangeCount,
} from "./turn-changed-files-helpers";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";

test.describe("mobile historical turn changed files", () => {
  test.describe.configure({ timeout: 120_000 });

  test("opens the exact captured turn in the full-height diff drawer", async ({
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

      await session.sendMessageViaButton("/e2e:untracked-file-modify");
      await expect(
        session.chat.getByText("untracked-file-modify complete", { exact: false }),
      ).toBeVisible();
      await session.waitForChatIdle();
      await waitForTurnChangeCount(apiClient, sessionId, 2);
      await expect(session.activeChat().getByTestId("turn-changed-files-card")).toHaveCount(2);
      if (prCapture.capturing) {
        const firstCard = session.activeChat().getByTestId("turn-changed-files-card").first();
        await firstCard.scrollIntoViewIfNeeded();
        await waitForFiniteAnimations(firstCard);
        await prCapture.screenshot("turn-changed-files-card-mobile", {
          caption: "Changed-files summary beneath its completed assistant turn on mobile",
        });
      }

      const git = new GitHelper(seedData.repositoryPath, makeGitEnv(backend.tmpDir));
      git.stageFile("untracked_test.txt");
      git.commit("retain later turn contents");

      const firstCard = await openFirstTurnChangesDiff(session);
      const drawer = testPage.getByTestId("mobile-diff-sheet");
      await expect(drawer).toBeVisible();
      if (prCapture.capturing) await waitForFiniteAnimations(drawer);
      const viewer = drawer.getByTestId("historical-turn-diff");
      await expect(viewer).toBeVisible();
      const changeSetId = await viewer.getAttribute("data-change-set-id");
      expect(changeSetId).toBeTruthy();
      const history = await readTurnChangeHistory(apiClient, sessionId);
      const latestTurn = [...(history.change_sets ?? [])].sort(
        (left, right) => right.turn_ordinal - left.turn_ordinal,
      )[0];
      expect(latestTurn).toBeDefined();
      expect(latestTurn!.id).not.toBe(changeSetId);
      await viewer.getByRole("combobox", { name: "Change scope" }).selectOption("latest");
      await expect(viewer).toHaveAttribute("data-change-set-id", latestTurn!.id);
      await viewer.getByRole("combobox", { name: "Change scope" }).selectOption(changeSetId!);
      await expect(viewer).toHaveAttribute("data-change-set-id", changeSetId!);
      await waitForDiffText(testPage, "INITIAL_CONTENT");
      await waitForDiffTextAbsent(testPage, "MODIFIED_CONTENT");
      if (prCapture.capturing) {
        await prCapture.screenshot("turn-changed-files-history-mobile", {
          caption: "Exact historical turn in the full-height mobile diff drawer",
        });
      }

      const close = drawer.getByTestId("mobile-diff-sheet-close");
      const closeBox = await close.boundingBox();
      expect(closeBox).not.toBeNull();
      expect(closeBox!.height).toBeGreaterThanOrEqual(44);
      await close.tap();
      await expect(drawer).toBeHidden();
      await expect(firstCard.getByRole("button", { name: "Open diff" })).toBeFocused();
    } finally {
      await restorePreference();
    }
  });
});
