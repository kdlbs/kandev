import { resetSeedRepositoryCheckout, test, expect } from "../../fixtures/test-base";
import { waitForDiffText, waitForDiffTextAbsent } from "./diff-update-helpers";
import {
  assertHistoricalSelectorStyle,
  enableTurnChangedFiles,
} from "./turn-changed-files-helpers";
import {
  TITLE_RENAME_BRANCH_PREFIX,
  TITLE_RENAME_CONTENT,
  TITLE_RENAME_FILE,
  TITLE_RENAME_LIVE_EDIT,
  TITLE_RENAME_SCENARIOS,
  assertNoTurnChangesCard,
  createTitleRenameTask,
  expectReadyRenameFileComparison,
  expectReadyRenameOnlyComparison,
  latestTitleRenameChangeSet,
  writeLiveWorktreeEdit,
} from "./turn-changes-title-rename-helpers";

test.use({ trace: "retain-on-failure" });

test.describe("mobile turn changes across a title-driven branch rename", () => {
  test.describe.configure({ timeout: 180_000 });

  test("rename-only turn has no changes card on the phone", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const restorePreference = await enableTurnChangedFiles(apiClient);
    try {
      resetSeedRepositoryCheckout(seedData, backend.tmpDir);
      const { session, sessionId, worktreeBranch } = await createTitleRenameTask(
        testPage,
        apiClient,
        seedData,
        TITLE_RENAME_SCENARIOS.only,
      );
      expect(worktreeBranch).toMatch(new RegExp(`^${TITLE_RENAME_BRANCH_PREFIX}-`));
      await expectReadyRenameOnlyComparison(apiClient, sessionId);

      await testPage.reload();
      await session.waitForLoad();
      await session.waitForChatIdle();
      await assertNoTurnChangesCard(session);
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      ).toBe(true);
    } finally {
      await restorePreference();
    }
  });

  test("renamed turn keeps its one-file historical diff in the phone drawer", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const restorePreference = await enableTurnChangedFiles(apiClient);
    try {
      resetSeedRepositoryCheckout(seedData, backend.tmpDir);
      const { session, sessionId, worktreePath, worktreeBranch } = await createTitleRenameTask(
        testPage,
        apiClient,
        seedData,
        TITLE_RENAME_SCENARIOS.edit,
      );
      expect(worktreeBranch).toMatch(new RegExp(`^${TITLE_RENAME_BRANCH_PREFIX}-`));
      await expectReadyRenameFileComparison(apiClient, sessionId);
      const { changeSet } = await latestTitleRenameChangeSet(apiClient, sessionId);

      await testPage.reload();
      await session.waitForLoad();
      await session.waitForChatIdle();
      const card = session.activeChat().getByTestId("turn-changed-files-card").last();
      await expect(card).toContainText("1 changed file");
      await expect(card).toContainText(TITLE_RENAME_FILE);
      await card.getByRole("button", { name: "Open diff" }).click();

      const drawer = testPage.getByTestId("mobile-diff-sheet");
      await expect(drawer).toBeVisible();
      const viewer = drawer.getByTestId("historical-turn-diff");
      await expect(viewer).toBeVisible();
      await expect(viewer).toHaveAttribute("data-change-set-id", changeSet.id);
      await assertHistoricalSelectorStyle(viewer, true);
      await waitForDiffText(testPage, TITLE_RENAME_CONTENT);
      expect(
        await drawer.evaluate((element) => element.scrollWidth <= element.clientWidth + 1),
      ).toBe(true);

      const close = drawer.getByTestId("mobile-diff-sheet-close");
      const closeBox = await close.boundingBox();
      expect(closeBox).not.toBeNull();
      expect(closeBox!.height).toBeGreaterThanOrEqual(44);
      await close.tap();
      await expect(drawer).toBeHidden();
      await expect(card.getByRole("button", { name: "Open diff" })).toBeFocused();

      writeLiveWorktreeEdit(worktreePath);
      await testPage.reload();
      await session.waitForLoad();
      await session.waitForChatIdle();
      const retained = session.activeChat().getByTestId("turn-changed-files-card").last();
      await expect(retained).toContainText("1 changed file");
      await retained.getByRole("button", { name: "Open diff" }).click();
      const retainedDrawer = testPage.getByTestId("mobile-diff-sheet");
      await expect(retainedDrawer).toBeVisible();
      await expect(retainedDrawer.getByTestId("historical-turn-diff")).toContainText(
        TITLE_RENAME_FILE,
      );
      await waitForDiffText(testPage, TITLE_RENAME_CONTENT);
      await waitForDiffTextAbsent(testPage, TITLE_RENAME_LIVE_EDIT);
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      ).toBe(true);
    } finally {
      await restorePreference();
    }
  });
});
