import { test, expect } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import {
  enableTurnChangedFiles,
  readTurnChangeHistory,
  seedTurnHistoryConflictTask,
  waitForTurnChangeCount,
} from "./turn-changed-files-helpers";

test.describe("turn change history conflict", () => {
  test.describe.configure({ timeout: 120_000 });

  test("shows one unavailable status without whole-repository totals", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const restorePreference = await enableTurnChangedFiles(apiClient);
    try {
      const { session, sessionId } = await seedTurnHistoryConflictTask(
        testPage,
        apiClient,
        seedData,
      );
      await waitForTurnChangeCount(apiClient, sessionId, 1);
      await session.sendMessage("/e2e:turn-history-conflict");
      await expect(
        session.chat.getByText("turn-history-conflict complete", { exact: false }),
      ).toBeVisible();
      await session.waitForChatIdle();
      await waitForTurnChangeCount(apiClient, sessionId, 2);

      const history = await readTurnChangeHistory(apiClient, sessionId);
      const turn = [...(history.change_sets ?? [])].sort(
        (left, right) => right.turn_ordinal - left.turn_ordinal,
      )[0];
      expect(turn?.availability).toBe("unavailable");
      expect(turn?.reason).toBe("history_conflict");
      expect(turn?.file_count).toBe(0);

      const card = session.activeChat().getByTestId("turn-changed-files-card").last();
      await expect(card).toBeVisible();
      await expect(card.getByText("This turn's change summary is unavailable.")).toHaveCount(1);
      await expect(card).not.toContainText("0 changed files");
      await expect(card).not.toContainText("Partial");
      await expect(card.getByRole("button", { name: "Open diff" })).toBeDisabled();
      if (prCapture.capturing) {
        await card.scrollIntoViewIfNeeded();
        await waitForFiniteAnimations(card);
        await prCapture.screenshot("turn-history-unavailable-desktop", {
          caption: "Conflicting baseline reuses the existing unavailable status",
        });
      }

      await testPage.reload();
      await session.waitForLoad();
      await session.waitForChatIdle();
      const reloaded = session.activeChat().getByTestId("turn-changed-files-card").last();
      await expect(reloaded.getByText("This turn's change summary is unavailable.")).toHaveCount(1);
    } finally {
      await restorePreference();
    }
  });
});
