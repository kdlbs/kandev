import { resetSeedRepositoryCheckout, test, expect } from "../../fixtures/test-base";
import { waitForDiffText, waitForDiffTextAbsent } from "./diff-update-helpers";
import { seedUntrackedFileTask } from "./diff-update-helpers";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import {
  assertHistoricalSelectorStyle,
  enableTurnChangedFiles,
  readTurnChangeHistory,
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
});
