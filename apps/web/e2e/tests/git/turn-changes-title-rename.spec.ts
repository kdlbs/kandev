import { resetSeedRepositoryCheckout, test, expect } from "../../fixtures/test-base";
import { waitForDiffText, waitForDiffTextAbsent } from "./diff-update-helpers";
import { enableTurnChangedFiles } from "./turn-changed-files-helpers";
import {
  TITLE_RENAME_BRANCH_PREFIX,
  TITLE_RENAME_CONTENT,
  TITLE_RENAME_FILE,
  TITLE_RENAME_LIVE_EDIT,
  TITLE_RENAME_SCENARIOS,
  TITLE_RENAME_SWITCH_BRANCH,
  assertNoTurnChangesCard,
  createTitleRenameTask,
  expectReadyRenameFileComparison,
  expectReadyRenameOnlyComparison,
  expectUnavailableRenameSwitch,
  writeLiveWorktreeEdit,
} from "./turn-changes-title-rename-helpers";

test.describe("turn changes across a title-driven branch rename", () => {
  test.describe.configure({ timeout: 180_000 });

  test("rename-only turn has no changes card", async ({
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

  test("renamed turn retains its one-file historical diff", async ({
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

      await testPage.reload();
      await session.waitForLoad();
      await session.waitForChatIdle();
      const card = session.activeChat().getByTestId("turn-changed-files-card").last();
      await expect(card).toContainText("1 changed file");
      await expect(card).toContainText(TITLE_RENAME_FILE);

      writeLiveWorktreeEdit(worktreePath);
      await testPage.reload();
      await session.waitForLoad();
      await session.waitForChatIdle();
      const retained = session.activeChat().getByTestId("turn-changed-files-card").last();
      await expect(retained).toContainText(TITLE_RENAME_FILE);
      await retained.getByRole("button", { name: "Open diff" }).click();
      await expect(testPage.getByTestId("historical-turn-diff")).toBeVisible();
      await waitForDiffText(testPage, TITLE_RENAME_CONTENT);
      await waitForDiffTextAbsent(testPage, TITLE_RENAME_LIVE_EDIT);
    } finally {
      await restorePreference();
    }
  });

  test("same-commit branch switch remains unavailable", async ({
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
        TITLE_RENAME_SCENARIOS.switch,
      );
      expect(
        worktreeBranch === TITLE_RENAME_SWITCH_BRANCH ||
          worktreeBranch.startsWith(`${TITLE_RENAME_BRANCH_PREFIX}-`),
      ).toBe(true);
      await expectUnavailableRenameSwitch(apiClient, sessionId);

      await testPage.reload();
      await session.waitForLoad();
      await session.waitForChatIdle();
      const cards = session.activeChat().getByTestId("turn-changed-files-card");
      await expect(cards).toHaveCount(1);
      await expect(cards.last()).toContainText("This turn's change summary is unavailable.");
    } finally {
      await restorePreference();
    }
  });
});
