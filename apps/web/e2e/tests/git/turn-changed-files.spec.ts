import { test, expect } from "../../fixtures/test-base";
import {
  seedUntrackedFileTask,
  waitForDiffText,
  waitForDiffTextAbsent,
} from "./diff-update-helpers";
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
      await assertLatestTurnCardInView(session);

      await session.sendMessage("/e2e:untracked-file-modify");
      await waitForTurnChangeCount(apiClient, sessionId, 2);
      await expect(
        session.chat.getByText("untracked-file-modify complete", { exact: false }),
      ).toBeVisible();
      await session.waitForChatIdle();

      const git = new GitHelper(seedData.repositoryPath, makeGitEnv(backend.tmpDir));
      git.stageFile("untracked_test.txt");
      git.commit("retain later turn contents");

      await testPage.reload();
      await session.waitForLoad();
      await session.waitForChatIdle();
      await expect(session.activeChat().getByTestId("turn-changed-files-card")).toHaveCount(2);
      await assertLatestTurnCardInView(session);
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
      await expect(firstCard.getByRole("button", { name: /Expand all|Collapse all/ })).toHaveCount(
        0,
      );
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
      await assertHistoricalSelectorStyle(viewer);
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

  test("leaves a 16px gap after anchored cards and keeps presentation aligned after reload", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const restorePreference = await enableTurnChangedFiles(apiClient);
    try {
      const { resetTurnQaFile } = await import("./turn-changed-files-qa-helpers");
      resetTurnQaFile(seedData.repositoryPath);
      const { session, sessionId } = await seedUntrackedFileTask(testPage, apiClient, seedData);
      await waitForTurnChangeCount(apiClient, sessionId, 1);

      const git = new GitHelper(seedData.repositoryPath, makeGitEnv(backend.tmpDir));
      git.stageFile("untracked_test.txt");
      git.commit("capture initial changed file");
      await session.sendMessage("/e2e:untracked-file-modify");
      await waitForTurnChangeCount(apiClient, sessionId, 2);
      await expect(
        session.chat.getByText("untracked-file-modify complete", { exact: false }),
      ).toBeVisible();
      await session.waitForChatIdle();

      let cards = session.activeChat().getByTestId("turn-changed-files-card");
      await expect(cards).toHaveCount(2);
      await assertTurnCardMessageGap(cards.first());
      await testPage.reload();
      await session.waitForLoad();
      await session.waitForChatIdle();
      cards = session.activeChat().getByTestId("turn-changed-files-card");
      await expect(cards).toHaveCount(2);
      await assertTurnCardMessageGap(cards.first());

      const secondCard = cards.nth(1);
      const fileRow = secondCard.locator("[data-turn-file-change-id]").first();
      await expect(fileRow).toContainText("Modified");
      const cardIcon = fileRow.locator("span[style*='font-family']");
      const cardIconDetails = await cardIcon.evaluate((element) => ({
        glyph: element.textContent,
        color: getComputedStyle(element).color,
      }));

      await session.clickTab("Files");
      const treeNode = session.fileTreeNode("untracked_test.txt");
      await expect(treeNode).toBeVisible();
      const treeIcon = treeNode.locator("span[style*='font-family']");
      await expect(treeIcon).toHaveCount(1);
      const treeIconDetails = await treeIcon.evaluate((element) => ({
        glyph: element.textContent,
        color: getComputedStyle(element).color,
      }));
      expect(cardIconDetails).toEqual(treeIconDetails);

      await session.clickTab("Changes");
      const changesRow = session.changes.locator('[data-changes-file="untracked_test.txt"]');
      await expect(changesRow).toBeVisible();
      const statusIcon = changesRow.locator('[data-file-status="modified"]');
      await expect(statusIcon).toHaveCount(1);
      const cardKind = fileRow.getByText("Modified", { exact: true });
      const cardAddedLines = fileRow.locator(".text-emerald-600").first();
      const panelAddedLines = changesRow.locator(".text-emerald-600").first();
      const cardRemovedLines = fileRow.locator(".text-rose-600").first();
      const panelRemovedLines = changesRow.locator(".text-rose-600").first();

      for (const colorScheme of ["light", "dark"] as const) {
        await testPage.emulateMedia({ colorScheme });
        const [
          kindColor,
          statusColor,
          cardAddedColor,
          panelAddedColor,
          cardRemovedColor,
          panelRemovedColor,
        ] = await Promise.all([
          cardKind.evaluate((element) => getComputedStyle(element).color),
          statusIcon.evaluate((element) => getComputedStyle(element).color),
          cardAddedLines.evaluate((element) => getComputedStyle(element).color),
          panelAddedLines.evaluate((element) => getComputedStyle(element).color),
          cardRemovedLines.evaluate((element) => getComputedStyle(element).color),
          panelRemovedLines.evaluate((element) => getComputedStyle(element).color),
        ]);
        expect(kindColor).toBe(statusColor);
        expect(cardAddedColor).toBe(panelAddedColor);
        expect(cardRemovedColor).toBe(panelRemovedColor);
      }
    } finally {
      await restorePreference();
    }
  });

  test("leaves a 16px gap after fallback cards", async ({ testPage, apiClient, seedData }) => {
    const restorePreference = await enableTurnChangedFiles(apiClient);
    try {
      const { resetTurnQaFile, routeTurnChangeFallback } =
        await import("./turn-changed-files-qa-helpers");
      resetTurnQaFile(seedData.repositoryPath);
      await routeTurnChangeFallback(testPage);
      const { session, sessionId } = await seedUntrackedFileTask(testPage, apiClient, seedData);
      await waitForTurnChangeCount(apiClient, sessionId, 1);
      await session.sendMessage("/e2e:untracked-file-modify");
      await waitForTurnChangeCount(apiClient, sessionId, 2);
      await session.waitForChatIdle();

      let fallbackCards = testPage
        .locator("[data-transcript-fallback-anchor]")
        .getByTestId("turn-changed-files-card");
      await expect(fallbackCards).toHaveCount(2);
      await assertTurnCardMessageGap(fallbackCards.first());
      await testPage.reload();
      await session.waitForLoad();
      await session.waitForChatIdle();
      fallbackCards = testPage
        .locator("[data-transcript-fallback-anchor]")
        .getByTestId("turn-changed-files-card");
      await expect(fallbackCards).toHaveCount(2);
      await assertTurnCardMessageGap(fallbackCards.first());
    } finally {
      await restorePreference();
    }
  });
});

test("recovers a failed file list instead of claiming no files", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const restorePreference = await enableTurnChangedFiles(apiClient);
  try {
    const { resetTurnQaFile } = await import("./turn-changed-files-qa-helpers");
    resetTurnQaFile(seedData.repositoryPath);
    const { sessionId } = await seedUntrackedFileTask(testPage, apiClient, seedData);
    await waitForTurnChangeCount(apiClient, sessionId, 1);
    await testPage.route("**/turn-changes/*/repositories/*/files?*", (route) =>
      route.fulfill({ status: 503, json: { error: "QA transient failure" } }),
    );
    const failedFiles = testPage.waitForResponse(
      (response) =>
        response.url().includes("/repositories/") &&
        response.url().includes("/files?") &&
        response.status() === 503,
    );
    await testPage.reload();
    await failedFiles;
    const card = testPage.getByTestId("turn-changed-files-card");
    await expect(card.getByRole("alert")).toBeVisible();
    await card.getByRole("button", { name: "Open diff", exact: true }).click();
    const viewer = testPage.getByTestId("historical-turn-diff");
    await expect(viewer.getByRole("alert")).toBeVisible();
    await testPage.unroute("**/turn-changes/*/repositories/*/files?*");
    await viewer.getByRole("button", { name: "Retry", exact: true }).click();
    await waitForDiffText(testPage, "INITIAL_CONTENT");
  } finally {
    await restorePreference();
  }
});

test("keeps renamed paths readable on a narrow fine-pointer viewport", async ({
  desktopTestPage: testPage,
  apiClient,
  seedData,
}) => {
  const restorePreference = await enableTurnChangedFiles(apiClient);
  try {
    await testPage.setViewportSize({ width: 390, height: 844 });
    expect(await testPage.evaluate(() => window.matchMedia("(pointer: fine)").matches)).toBe(true);
    const {
      routeRenamedTurnFile,
      resetTurnQaFile,
      assertCardResponsiveContainment,
      assertCardTouchTargets,
      assertTurnFileDisclosure,
    } = await import("./turn-changed-files-qa-helpers");
    await routeRenamedTurnFile(testPage);
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
    await expandAll.click();
    await expect(card.locator("[data-turn-file-change-id]")).toHaveCount(1);
    await assertCardResponsiveContainment(testPage, card);
    await assertCardTouchTargets(testPage);
    await assertTurnFileDisclosure(testPage);
    await testPage.screenshot({
      path: test.info().outputPath("renamed-path-narrow-fine-pointer.png"),
    });
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
    await prCapture.screenshot("empty-capture-desktop", {
      caption: "Completed reply without an empty unavailable change card",
    });
  } finally {
    await restorePreference();
  }
});
