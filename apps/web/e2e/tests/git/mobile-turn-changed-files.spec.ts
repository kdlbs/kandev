import { test, expect } from "../../fixtures/test-base";
import { waitForDiffText, waitForDiffTextAbsent } from "./diff-update-helpers";
import { seedUntrackedFileTask } from "./diff-update-helpers";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import {
  assertHistoricalSelectorStyle,
  assertLatestTurnCardInView,
  assertTurnCardMessageGap,
  enableTurnChangedFiles,
  openFirstTurnChangesDiff,
  readTurnChangeHistory,
  waitForTurnChangeCount,
} from "./turn-changed-files-helpers";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";

test.use({ trace: "retain-on-failure" });

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
      // The page can load the summary while capture is still pending. Rehydrate the
      // finalized history after the backend readiness check before opening the card.
      await testPage.reload();
      await session.waitForLoad();
      await session.waitForChatIdle();
      await expect(session.activeChat().getByTestId("turn-changed-files-card")).toHaveCount(1);
      await assertLatestTurnCardInView(session);

      await session.sendMessageViaButton("/e2e:untracked-file-modify");
      await waitForTurnChangeCount(apiClient, sessionId, 2);
      await expect(
        session.chat.getByText("untracked-file-modify complete", { exact: false }),
      ).toBeVisible();
      await session.waitForChatIdle();
      await expect(session.activeChat().getByTestId("turn-changed-files-card")).toHaveCount(2);
      const firstCard = session.activeChat().getByTestId("turn-changed-files-card").first();
      await assertTurnCardMessageGap(firstCard);
      if (prCapture.capturing) {
        await firstCard.scrollIntoViewIfNeeded();
        await waitForFiniteAnimations(firstCard);
        await prCapture.screenshot("turn-changed-files-card-mobile", {
          caption: "Changed-files summary beneath its completed assistant turn on mobile",
        });
      }

      await testPage.reload();
      await session.waitForLoad();
      await session.waitForChatIdle();
      const reloadedCards = session.activeChat().getByTestId("turn-changed-files-card");
      await expect(reloadedCards).toHaveCount(2);
      await assertTurnCardMessageGap(reloadedCards.first());

      const git = new GitHelper(seedData.repositoryPath, makeGitEnv(backend.tmpDir));
      git.stageFile("untracked_test.txt");
      git.commit("retain later turn contents");

      await openFirstTurnChangesDiff(session);
      const drawer = testPage.getByTestId("mobile-diff-sheet");
      await expect(drawer).toBeVisible();
      if (prCapture.capturing) await waitForFiniteAnimations(drawer);
      const viewer = drawer.getByTestId("historical-turn-diff");
      await expect(viewer).toBeVisible();
      await assertHistoricalSelectorStyle(viewer, true);
      await waitForDiffText(testPage, "INITIAL_CONTENT");
      await testPage.screenshot({ path: test.info().outputPath("historical-selectors.png") });
      await testPage.emulateMedia({ colorScheme: "dark" });
      await expect(testPage.locator("html")).toHaveClass(/dark/);
      await expect
        .poll(() =>
          viewer
            .locator("diffs-container")
            .first()
            .evaluate((element) => getComputedStyle(element).colorScheme),
        )
        .toBe("dark");
      await testPage.screenshot({ path: test.info().outputPath("historical-selectors-dark.png") });
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

test("keeps renamed file paths readable in the phone diff", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const restorePreference = await enableTurnChangedFiles(apiClient);
  try {
    const {
      routeRenamedTurnFile,
      assertCardResponsiveContainment,
      assertCardTouchTargets,
      assertLongFileMetadataWraps,
      assertTurnFileDisclosure,
    } = await import("./turn-changed-files-qa-helpers");
    await routeRenamedTurnFile(testPage);
    const { resetTurnQaFile } = await import("./turn-changed-files-qa-helpers");
    resetTurnQaFile(seedData.repositoryPath);
    const fileTreeLoaded = testPage.waitForResponse(
      (response) =>
        response.url().includes("/repositories/") &&
        response.url().includes("/files?") &&
        response.status() === 200,
    );
    const { sessionId } = await seedUntrackedFileTask(testPage, apiClient, seedData);
    await waitForTurnChangeCount(apiClient, sessionId, 1);
    await fileTreeLoaded;
    const card = testPage.getByTestId("turn-changed-files-card");
    await expect(card).toBeVisible();
    const expandAll = card.getByRole("button", { name: "Expand all", exact: true });
    await expect(expandAll).toBeVisible();
    await expandAll.tap();
    await expect(card.locator("[data-turn-file-change-id]")).toHaveCount(1);
    for (const width of [390, 767, 768]) {
      await testPage.setViewportSize({ width, height: 844 });
      await assertCardResponsiveContainment(testPage, card);
      if (width === 390) await assertCardTouchTargets(testPage);
    }
    await testPage.setViewportSize({ width: 390, height: 844 });
    await assertLongFileMetadataWraps(testPage);
    await assertTurnFileDisclosure(testPage, true);
    await testPage.screenshot({ path: test.info().outputPath("renamed-path-phone.png") });
  } finally {
    await restorePreference();
  }
});

test("hides an unavailable empty capture after reload", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const restorePreference = await enableTurnChangedFiles(apiClient);
  try {
    const { resetTurnQaFile, routeEmptyUnavailableTurn } =
      await import("./turn-changed-files-qa-helpers");
    const { waitForHttp } = await import("../../helpers/causal-waits");
    resetTurnQaFile(seedData.repositoryPath);
    const { session, sessionId } = await seedUntrackedFileTask(testPage, apiClient, seedData);
    await waitForTurnChangeCount(apiClient, sessionId, 1);
    // Confirm the completed capture is visible before intercepting its persisted reload.
    await testPage.reload();
    await session.waitForLoad();
    await session.waitForChatIdle();
    await expect(session.activeChat().getByTestId("turn-changed-files-card")).toHaveCount(1);
    await routeEmptyUnavailableTurn(testPage, sessionId);
    const historyLoaded = waitForHttp(testPage, "GET", /\/turn-changes$/);
    await testPage.reload();
    const response = await historyLoaded;
    expect((await response.json()).change_sets[0].file_count).toBe(0);
    await session.waitForLoad();
    await expect(
      session.chat.getByText("untracked-file-setup complete", { exact: false }),
    ).toBeVisible();
    await session.waitForChatIdle();
    await expect(session.activeChat().getByTestId("turn-changed-files-card")).toHaveCount(0);
    await prCapture.screenshot("empty-capture-mobile", {
      caption: "Completed reply without an empty unavailable change card",
    });
  } finally {
    await restorePreference();
  }
});
