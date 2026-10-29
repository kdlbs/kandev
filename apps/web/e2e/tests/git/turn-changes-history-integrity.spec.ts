import { resetSeedRepositoryCheckout, test, expect } from "../../fixtures/test-base";
import { waitForDiffText, waitForDiffTextAbsent } from "./diff-update-helpers";
import { seedUntrackedFileTask } from "./diff-update-helpers";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import {
  enableTurnChangedFiles,
  readTurnChangeHistory,
  waitForTurnChangeCount,
} from "./turn-changed-files-helpers";

test.describe("turn change history integrity", () => {
  test.describe.configure({ timeout: 120_000 });

  test("excludes imported upstream history and keeps the local turn file", async ({
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
      await session.sendMessage("/e2e:turn-history-integrity");
      await expect(
        session.chat.getByText("turn-history-integrity complete", { exact: false }),
      ).toBeVisible();
      await session.waitForChatIdle();
      await waitForTurnChangeCount(apiClient, sessionId, 2);

      const history = await readTurnChangeHistory(apiClient, sessionId);
      const turn = [...(history.change_sets ?? [])].sort(
        (left, right) => right.turn_ordinal - left.turn_ordinal,
      )[0];
      expect(turn).toBeDefined();
      expect(turn!.file_count).toBe(1);
      expect(turn!.repositories?.[0]?.comparison_basis).toBe("upstream_adjusted");

      const cards = session.activeChat().getByTestId("turn-changed-files-card");
      await expect(cards).toHaveCount(2);
      const card = cards.last();
      await expect(card).toContainText("1 changed file");
      await expect(card).toContainText("turn-history.txt");
      if (prCapture.capturing) {
        await card.scrollIntoViewIfNeeded();
        await waitForFiniteAnimations(card);
        await prCapture.screenshot("turn-history-card-desktop", {
          caption: "Corrected historical turn summary after an upstream rebase",
        });
      }

      await testPage.reload();
      await session.waitForLoad();
      await session.waitForChatIdle();
      const reloaded = session.activeChat().getByTestId("turn-changed-files-card").last();
      await expect(reloaded).toContainText("1 changed file");
      const reloadedHistory = await readTurnChangeHistory(apiClient, sessionId);
      const sameTurn = (reloadedHistory.change_sets ?? []).find((item) => item.id === turn!.id);
      expect(sameTurn?.file_count).toBe(1);
      expect(sameTurn?.repositories?.[0]?.comparison_basis).toBe("upstream_adjusted");

      await reloaded.getByRole("button", { name: "Open diff" }).click();
      const viewer = testPage.getByTestId("historical-turn-diff");
      await expect(viewer).toBeVisible();
      await waitForDiffText(testPage, "local turn change");
      await waitForDiffTextAbsent(testPage, "integrated upstream change");
      await expect(viewer).not.toContainText("untracked-before.txt");
      if (prCapture.capturing) {
        await waitForFiniteAnimations(viewer);
        await prCapture.screenshot("turn-history-viewer-desktop", {
          caption: "Historical diff keeps only the local turn change",
        });
      }
    } finally {
      await restorePreference();
    }
  });
});
