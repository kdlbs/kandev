import { test, expect } from "../../fixtures/test-base";
import { dwell, waitForHttp, watchWs } from "../../helpers/causal-waits";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import { waitForAgentMessage } from "../../helpers/session";
import {
  enableTurnChangedFiles,
  readTurnChangeHistory,
  seedTurnHistoryConflictTask,
  waitForTurnChangeCount,
} from "./turn-changed-files-helpers";

test.describe("turn change history conflict", () => {
  test.describe.configure({ timeout: 120_000 });

  test("hides the unavailable comparison and keeps the assistant reply", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const restorePreference = await enableTurnChangedFiles(apiClient);
    const ws = watchWs(testPage);
    try {
      const { session, sessionId } = await seedTurnHistoryConflictTask(
        testPage,
        apiClient,
        seedData,
      );
      await waitForTurnChangeCount(apiClient, sessionId, 1);
      const unavailableSummary = ws.waitForEvent("session.turn.changes.updated", {
        where: (payload) => {
          const changeSet = payload.change_set as { availability?: string } | undefined;
          return payload.session_id === sessionId && changeSet?.availability === "unavailable";
        },
      });
      await session.sendMessage("/e2e:turn-history-conflict");
      await waitForAgentMessage(apiClient, sessionId, "turn-history-conflict complete");
      const reply = session.chat.getByText("turn-history-conflict complete", { exact: false });
      await expect(reply).toBeVisible();
      await session.waitForChatIdle();
      await waitForTurnChangeCount(apiClient, sessionId, 2);
      await unavailableSummary;

      const history = await readTurnChangeHistory(apiClient, sessionId);
      const turn = [...(history.change_sets ?? [])].sort(
        (left, right) => right.turn_ordinal - left.turn_ordinal,
      )[0];
      expect(turn?.availability).toBe("unavailable");
      expect(turn?.reason).toBe("history_conflict");
      expect(turn?.file_count).toBe(0);

      const chat = session.activeChat();
      await expect(chat.locator(`[data-change-set-id="${turn!.id}"]`)).toHaveCount(0);
      await expect(chat.getByText("This turn's change summary is unavailable.")).toHaveCount(0);
      await dwell(
        testPage,
        500,
        "negative-assertion",
        "an unavailable history summary must never render a transcript card",
      );
      await expect(chat.locator(`[data-change-set-id="${turn!.id}"]`)).toHaveCount(0);
      if (prCapture.capturing) {
        await reply.scrollIntoViewIfNeeded();
        await waitForFiniteAnimations(reply);
        await prCapture.screenshot("turn-history-unavailable-desktop", {
          caption: "Unavailable history comparison omits the turn card",
        });
      }

      const historyLoaded = waitForHttp(testPage, "GET", /\/turn-changes$/);
      await testPage.reload();
      await historyLoaded;
      await session.waitForLoad();
      await session.waitForChatIdle();
      const reloadedChat = session.activeChat();
      await expect(
        reloadedChat.getByText("turn-history-conflict complete", { exact: false }),
      ).toBeVisible();
      await expect(reloadedChat.locator(`[data-change-set-id="${turn!.id}"]`)).toHaveCount(0);
    } finally {
      await restorePreference();
    }
  });
});
