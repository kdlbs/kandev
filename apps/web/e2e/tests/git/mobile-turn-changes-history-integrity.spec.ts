import { resetSeedRepositoryCheckout, test, expect } from "../../fixtures/test-base";
import { waitForDiffText, waitForDiffTextAbsent } from "./diff-update-helpers";
import { seedUntrackedFileTask } from "./diff-update-helpers";
import { dwell, waitForHttp, watchWs } from "../../helpers/causal-waits";
import { waitForAgentMessage } from "../../helpers/session";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import {
  assertHistoricalSelectorStyle,
  enableTurnChangedFiles,
  readTurnChangeHistory,
  seedTurnHistoryConflictTask,
  waitForTurnChangeCount,
} from "./turn-changed-files-helpers";

test.use({ trace: "retain-on-failure" });

test.describe("mobile turn change history integrity", () => {
  test.describe.configure({ timeout: 120_000 });

  test("opens the corrected historical turn in the phone diff drawer", async ({
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
      await session.sendMessageViaButton("/e2e:turn-history-integrity");
      await expect(
        session.chat.getByText("turn-history-integrity complete", { exact: false }),
      ).toBeVisible();
      await session.waitForChatIdle();
      await waitForTurnChangeCount(apiClient, sessionId, 2);

      const history = await readTurnChangeHistory(apiClient, sessionId);
      const turn = [...(history.change_sets ?? [])].sort(
        (left, right) => right.turn_ordinal - left.turn_ordinal,
      )[0];
      expect(turn?.file_count).toBe(1);
      expect(turn?.repositories?.[0]?.comparison_basis).toBe("upstream_adjusted");

      const card = session.activeChat().getByTestId("turn-changed-files-card").last();
      await expect(card).toContainText("1 changed file");
      await expect(card).toContainText("turn-history.txt");
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      ).toBe(true);
      if (prCapture.capturing) {
        await card.scrollIntoViewIfNeeded();
        await waitForFiniteAnimations(card);
        await prCapture.screenshot("turn-history-card-phone", {
          caption: "Phone summary keeps the corrected historical turn",
        });
      }

      await testPage.reload();
      await session.waitForLoad();
      await session.waitForChatIdle();
      const reloaded = session.activeChat().getByTestId("turn-changed-files-card").last();
      await expect(reloaded).toContainText("1 changed file");
      await testPage.emulateMedia({ colorScheme: "dark" });
      await expect(testPage.locator("html")).toHaveClass(/dark/);
      await reloaded.getByRole("button", { name: "Open diff" }).click();
      const drawer = testPage.getByTestId("mobile-diff-sheet");
      await expect(drawer).toBeVisible();
      const viewer = drawer.getByTestId("historical-turn-diff");
      await expect(viewer).toBeVisible();
      await assertHistoricalSelectorStyle(viewer, true);
      await waitForDiffText(testPage, "local turn change");
      if (prCapture.capturing) {
        await waitForFiniteAnimations(viewer);
        await prCapture.screenshot("turn-history-viewer-phone", {
          caption: "Phone historical diff keeps the local turn change",
        });
      }
      await waitForDiffTextAbsent(testPage, "integrated upstream change");
    } finally {
      await restorePreference();
    }
  });

  test("keeps the pseudo-localized phone card and drawer contained", async ({
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
      await testPage.evaluate(() => {
        document.cookie = "kandev_locale=pseudo; path=/; max-age=31536000; SameSite=Lax";
      });
      await testPage.reload();
      await session.waitForLoad();
      await expect(testPage.locator("html")).toHaveAttribute("lang", "pseudo", { timeout: 15_000 });

      const card = session.activeChat().getByTestId("turn-changed-files-card").first();
      await expect(card).toContainText("ćĥàńĝēď ƒĩĺē");
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      ).toBe(true);

      await card.getByRole("button", { name: /Ōƥēń ďĩƒƒ/ }).click();
      const drawer = testPage.getByTestId("mobile-diff-sheet");
      await expect(drawer).toBeVisible();
      const viewer = drawer.getByTestId("historical-turn-diff");
      await expect(viewer).toBeVisible();
      expect(
        await drawer.evaluate((element) => element.scrollWidth <= element.clientWidth + 1),
      ).toBe(true);
      await waitForDiffText(testPage, "INITIAL_CONTENT");
    } finally {
      await testPage
        .evaluate(() => {
          document.cookie = "kandev_locale=en; path=/; max-age=0; SameSite=Lax";
        })
        .catch(() => undefined);
      await restorePreference();
    }
  });

  test("hides an unavailable history comparison before and after reload", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    const restorePreference = await enableTurnChangedFiles(apiClient);
    const ws = watchWs(testPage);
    try {
      resetSeedRepositoryCheckout(seedData, backend.tmpDir);
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
      await session.sendMessageViaButton("/e2e:turn-history-conflict");
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

      const chat = session.activeChat();
      await expect(chat.locator(`[data-change-set-id="${turn!.id}"]`)).toHaveCount(0);
      await dwell(
        testPage,
        500,
        "negative-assertion",
        "an unavailable history summary must never render a phone transcript card",
      );
      await expect(chat.locator(`[data-change-set-id="${turn!.id}"]`)).toHaveCount(0);
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      ).toBe(true);
      if (prCapture.capturing) {
        await reply.scrollIntoViewIfNeeded();
        await waitForFiniteAnimations(reply);
        await prCapture.screenshot("turn-history-unavailable-phone", {
          caption: "Phone unavailable history comparison omits the turn card",
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
