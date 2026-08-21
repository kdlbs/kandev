import path from "node:path";
import { test, expect, type SeedData } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { BackendContext } from "../../fixtures/backend";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";

async function setupMobileFileTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  backend: BackendContext,
) {
  const suffix = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
  const filePath = `mobile-delete-file-${suffix}.md`;
  const git = new GitHelper(
    path.join(backend.tmpDir, "repos", "e2e-repo"),
    makeGitEnv(backend.tmpDir),
  );
  const originalSha = git.getCurrentSha();
  const restoreRepository = () => git.exec(`git reset --hard ${originalSha}`);

  try {
    git.createFile(filePath, "mobile delete confirmation\n");
    git.stageAll();
    git.commit(`add mobile delete fixture ${suffix}`);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile file confirmations",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 45_000 });
    return { session, filePath, restoreRepository };
  } catch (error) {
    restoreRepository();
    throw error;
  }
}

let restoreRepository: (() => void) | undefined;

test.describe("Mobile file confirmations", () => {
  test.afterEach(() => {
    restoreRepository?.();
    restoreRepository = undefined;
  });

  test("confirms one file deletion inline without horizontal overflow", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    test.setTimeout(90_000);
    const fixture = await setupMobileFileTask(testPage, apiClient, seedData, backend);
    const { session, filePath } = fixture;
    restoreRepository = fixture.restoreRepository;

    expect(await testPage.evaluate(() => matchMedia("(any-pointer: coarse)").matches)).toBe(true);
    await testPage.getByRole("button", { name: "Files" }).tap();

    const file = session.fileTreeNode(filePath);
    await expect(file).toBeVisible({ timeout: 15_000 });
    const trigger = session.fileTreeNodeActions(filePath);
    await expect(trigger).toBeVisible();
    const triggerBox = await trigger.boundingBox();
    expect(triggerBox).not.toBeNull();
    expect(triggerBox!.width).toBeGreaterThanOrEqual(44);
    expect(triggerBox!.height).toBeGreaterThanOrEqual(44);

    await trigger.tap();
    const menu = session.fileTreeTouchMenu();
    await expect(menu).toBeVisible();
    const deleteItem = testPage.getByTestId("file-tree-touch-delete");
    await expect(deleteItem).toBeVisible();
    await expect
      .poll(async () => (await deleteItem.boundingBox())?.height ?? 0, { timeout: 2_000 })
      .toBeGreaterThanOrEqual(44);

    const viewport = await testPage.evaluate(() => ({
      width: window.innerWidth,
      height: window.innerHeight,
    }));
    const menuBox = await menu.boundingBox();
    expect(menuBox).not.toBeNull();
    expect(menuBox!.x).toBeGreaterThanOrEqual(0);
    expect(menuBox!.x + menuBox!.width).toBeLessThanOrEqual(viewport.width);
    expect(menuBox!.y).toBeGreaterThanOrEqual(0);
    expect(menuBox!.y + menuBox!.height).toBeLessThanOrEqual(viewport.height);

    await deleteItem.tap();
    const inline = testPage.getByTestId("file-delete-inline-confirmation");
    await expect(inline).toBeVisible();
    await expect(testPage.getByRole("alertdialog")).toHaveCount(0);
    const confirm = inline.getByTestId("file-delete-confirm");
    const cancel = inline.getByRole("button", { name: "Cancel" });
    await expect(confirm).toBeVisible();
    await expect(cancel).toBeVisible();
    const inlineBox = await inline.boundingBox();
    expect(inlineBox).not.toBeNull();
    expect(
      inlineBox!.x,
      `inline box: ${JSON.stringify(inlineBox)}, viewport: ${viewport.width}`,
    ).toBeGreaterThanOrEqual(0);
    expect(
      inlineBox!.x + inlineBox!.width,
      `inline box: ${JSON.stringify(inlineBox)}, viewport: ${viewport.width}`,
    ).toBeLessThanOrEqual(viewport.width);
    for (const button of [cancel, confirm]) {
      const buttonBox = await button.boundingBox();
      expect(buttonBox).not.toBeNull();
      expect(buttonBox!.x).toBeGreaterThanOrEqual(0);
      expect(buttonBox!.x + buttonBox!.width).toBeLessThanOrEqual(viewport.width);
    }
    await expect
      .poll(async () => (await confirm.boundingBox())?.height ?? 0, { timeout: 2_000 })
      .toBeGreaterThanOrEqual(44);
    await expect
      .poll(async () => (await cancel.boundingBox())?.height ?? 0, { timeout: 2_000 })
      .toBeGreaterThanOrEqual(44);

    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
    await prCapture.screenshot("mobile-file-delete-confirmation", {
      caption: "Pixel 5 file actions keep one-file deletion confirmation inline and touch-safe",
    });

    await confirm.tap();
    await expect(file).not.toBeVisible({ timeout: 15_000 });
  });
});
