import { expect, test } from "../../fixtures/test-base";
import type { Locator, Page } from "@playwright/test";
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";

async function chooseDirectory(
  page: Page,
  trigger: Locator,
  pickerRoot: string,
  directory: string,
) {
  const relativeDirectory = path.relative(pickerRoot, directory);
  if (relativeDirectory === ".." || relativeDirectory.startsWith(`..${path.sep}`)) {
    throw new Error(`Directory ${directory} is outside picker root ${pickerRoot}`);
  }
  await trigger.tap();
  const picker = page.locator('[data-testid="folder-picker-popover"][data-state="open"]');
  await expect(picker).toBeVisible();
  for (const segment of relativeDirectory.split(path.sep).filter(Boolean)) {
    await picker.getByRole("button", { name: segment, exact: true }).tap();
  }
  await picker.getByTestId("folder-picker-choose").tap();
}

test("mobile Files drawer attaches sources with fixed controls and persisted workspace", async ({
  testPage,
  apiClient,
  seedData,
  backend,
  prCapture,
}) => {
  test.setTimeout(120_000);
  const gitEnv = makeGitEnv(backend.tmpDir);
  const repositoryPath = path.join(backend.tmpDir, "mobile-sources", "mobile-local-repository");
  const originPath = path.join(
    backend.tmpDir,
    "mobile-sources",
    "mobile-local-repository-origin.git",
  );
  const folderPath = path.join(backend.tmpDir, "mobile-sources", "mobile-local-folder");
  fs.mkdirSync(repositoryPath, { recursive: true });
  fs.mkdirSync(folderPath, { recursive: true });
  fs.writeFileSync(path.join(repositoryPath, "mobile-repository.txt"), "repository source\n");
  fs.writeFileSync(path.join(folderPath, "mobile-folder.txt"), "folder source\n");
  execFileSync("git", ["init", "-b", "main"], { cwd: repositoryPath, env: gitEnv });
  execFileSync("git", ["add", "."], { cwd: repositoryPath, env: gitEnv });
  execFileSync("git", ["commit", "-m", "initial source"], { cwd: repositoryPath, env: gitEnv });
  // Source worktrees perform a required refresh. Use a local bare origin so
  // this fixture remains offline while satisfying that production contract.
  execFileSync("git", ["init", "--bare", "-b", "main", originPath], { env: gitEnv });
  execFileSync("git", ["remote", "add", "origin", originPath], {
    cwd: repositoryPath,
    env: gitEnv,
  });
  execFileSync("git", ["push", "--set-upstream", "origin", "main"], {
    cwd: repositoryPath,
    env: gitEnv,
  });
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile attach local sources",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      executor_profile_id: seedData.worktreeExecutorProfileId,
    },
  );

  // The "Add folder" control is gated on the task's primary executor binding.
  // Poll the backend directly for it rather than budgeting the later UI
  // assertion for the async session launch that produces it. The budget sits on
  // the backend cause, not on a UI shadow of it, so the "Add folder" assertion
  // below keeps the default timeout. `primary_executor_type` is `omitempty`, so
  // an unbound task omits the key entirely: assert truthiness, never
  // `not.toBeNull()`, which `undefined` satisfies on the first poll.
  await expect
    .poll(async () => (await apiClient.getTask(task.id)).primary_executor_type, {
      timeout: 30_000,
    })
    .toBeTruthy();

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  await testPage.getByRole("button", { name: "Files" }).tap();
  const entryPoint = testPage.getByTestId("files-workspace-actions");
  await expect(entryPoint).toBeVisible();
  await expect(entryPoint).toBeEnabled();
  const entryBox = await entryPoint.boundingBox();
  expect(entryBox).not.toBeNull();
  expect(entryBox!.height).toBeGreaterThanOrEqual(44);
  expect(entryBox!.width).toBeGreaterThanOrEqual(44);
  expect(entryBox!.width).toBeLessThanOrEqual(48);
  await entryPoint.tap();
  const addSources = testPage.getByRole("menuitem", {
    name: "Add Repositories to workspace",
  });
  const openFolder = testPage.getByRole("menuitem", { name: "Open workspace folder" });
  const actionMenu = addSources.locator("xpath=ancestor::*[@role='menu'][1]");
  await actionMenu.evaluate((element) =>
    Promise.all(
      element
        .getAnimations({ subtree: true })
        .map((animation) => animation.finished.catch(() => undefined)),
    ),
  );
  const [menuBox, addSourcesBox, openFolderBox] = await Promise.all([
    actionMenu.boundingBox(),
    addSources.boundingBox(),
    openFolder.boundingBox(),
  ]);
  const menuViewport = testPage.viewportSize();
  if (!menuBox || !addSourcesBox || !openFolderBox || !menuViewport) {
    throw new Error("mobile workspace actions menu has no layout box");
  }
  expect(menuBox.x).toBeGreaterThanOrEqual(8);
  expect(menuBox.x + menuBox.width).toBeLessThanOrEqual(menuViewport.width - 8);
  expect(menuViewport.height - (menuBox.y + menuBox.height)).toBeGreaterThanOrEqual(7);
  expect(addSourcesBox.height).toBeGreaterThanOrEqual(44);
  expect(openFolderBox.height).toBeGreaterThanOrEqual(44);
  if (!task.session_id) throw new Error("task creation did not return a session id");
  await Promise.all([
    testPage.waitForRequest(
      (request) =>
        request.method() === "POST" &&
        request.url().endsWith(`/api/v1/task-sessions/${task.session_id}/open-folder`),
    ),
    openFolder.tap(),
  ]);
  await expect(openFolder).not.toBeVisible();
  await entryPoint.tap();
  await testPage.getByRole("menuitem", { name: "Add Repositories to workspace" }).tap();

  const drawer = testPage.getByTestId("add-workspace-sources-drawer");
  await expect(drawer).toBeVisible();
  const consequences = drawer.getByTestId("workspace-change-consequences");
  await expect(consequences).toBeVisible();
  await expect(consequences).toContainText("This restarts the task workspace");
  await expect(consequences).toContainText("Cancel leaves the workspace unchanged");
  await expect(consequences).toContainText(
    "terminals, dev servers, and other workspace processes stop",
  );
  const fullImpactDetails = consequences.getByText("Full impact details");
  const fullImpactDetailsBox = await fullImpactDetails.boundingBox();
  expect(fullImpactDetailsBox?.height).toBeGreaterThanOrEqual(44);
  await fullImpactDetails.tap();
  await expect(
    consequences.getByText(/The task root becomes the agent's working directory/),
  ).toBeVisible();
  await fullImpactDetails.tap();
  const [drawerBox, viewport] = await Promise.all([
    drawer.boundingBox(),
    testPage.evaluate(() => ({ width: innerWidth, height: innerHeight })),
  ]);
  expect(drawerBox).not.toBeNull();
  expect(drawerBox!.height).toBeGreaterThanOrEqual(viewport.height - 1);
  await expect(testPage.locator("html")).toHaveJSProperty("scrollWidth", viewport.width);
  const scrollOwners = await drawer.locator("*").evaluateAll(
    (elements) =>
      elements.filter((element) => {
        const overflowY = getComputedStyle(element).overflowY;
        return overflowY === "auto" || overflowY === "scroll";
      }).length,
  );
  expect(scrollOwners).toBe(1);

  await expect(drawer.getByTestId("source-mode-local")).toHaveCount(0);
  await expect(drawer.getByTestId("source-mode-remote")).toHaveCount(0);
  const addRepository = drawer.getByRole("button", { name: "Add repository" });
  const addFolder = drawer.getByRole("button", { name: "Add folder" });
  const submit = drawer.getByTestId("add-workspace-sources-submit");
  // The "Add folder" control is gated on activeTask.primaryExecutorType. The
  // poll before navigation already confirmed the backend has resolved it, so
  // the drawer renders it on first paint and this needs no extended budget.
  await expect(addRepository).toBeVisible();
  await expect(addFolder).toBeVisible();
  const [addRepositoryBox, addFolderBox] = await Promise.all([
    addRepository.boundingBox(),
    addFolder.boundingBox(),
  ]);
  expect(addRepositoryBox?.height).toBeGreaterThanOrEqual(44);
  expect(addFolderBox?.height).toBeGreaterThanOrEqual(44);
  await expect(submit).toBeDisabled();
  await addRepository.tap();
  const workspaceRepository = testPage.getByRole("menuitem", { name: "Workspace repository" });
  const localRepository = testPage.getByRole("menuitem", { name: "Local Git repository" });
  const remoteRepository = testPage.getByRole("menuitem", { name: "Remote repository" });
  const repositoryMenu = workspaceRepository.locator("xpath=ancestor::*[@role='menu'][1]");
  await repositoryMenu.evaluate((element) =>
    Promise.all(
      element
        .getAnimations({ subtree: true })
        .map((animation) => animation.finished.catch(() => undefined)),
    ),
  );
  const [workspaceRepositoryBox, localRepositoryBox, remoteRepositoryBox] = await Promise.all([
    workspaceRepository.boundingBox(),
    localRepository.boundingBox(),
    remoteRepository.boundingBox(),
  ]);
  expect(workspaceRepositoryBox?.height).toBeGreaterThanOrEqual(44);
  expect(localRepositoryBox?.height).toBeGreaterThanOrEqual(44);
  expect(remoteRepositoryBox?.height).toBeGreaterThanOrEqual(44);
  await workspaceRepository.tap();
  const savedRepositoryRow = drawer.getByTestId("workspace-source-row").first();
  await expect(savedRepositoryRow.getByRole("alert")).toHaveCount(0);
  await expect(savedRepositoryRow.getByRole("textbox", { name: "Checkout branch" })).toHaveCount(0);
  await savedRepositoryRow.getByTestId("repo-chip-trigger").tap();
  const refreshRepositories = testPage.getByTestId("repo-refresh-button");
  const createRepository = testPage.getByTestId("create-local-repository-button");
  await expect(refreshRepositories).toBeVisible();
  await expect(createRepository).toBeVisible();
  const [refreshBox, createBox] = await Promise.all([
    refreshRepositories.boundingBox(),
    createRepository.boundingBox(),
  ]);
  expect(refreshBox?.height).toBeGreaterThanOrEqual(44);
  expect(createBox?.height).toBeGreaterThanOrEqual(44);
  await testPage.keyboard.press("Escape");
  await savedRepositoryRow.getByRole("button", { name: "Remove source" }).tap();
  await addRepository.tap();
  await testPage.getByRole("menuitem", { name: "Local Git repository" }).tap();
  await addFolder.tap();
  const rows = drawer.getByTestId("workspace-source-row");
  await expect(rows).toHaveCount(2);
  await chooseDirectory(
    testPage,
    rows.nth(0).getByTestId("folder-picker-trigger"),
    backend.tmpDir,
    repositoryPath,
  );
  await rows.nth(0).getByRole("textbox", { name: "Base branch" }).fill("main");
  await chooseDirectory(
    testPage,
    rows.nth(1).getByTestId("folder-picker-trigger"),
    backend.tmpDir,
    folderPath,
  );
  await expect(rows).toHaveCount(2);
  await prCapture.screenshot("workspace-actions-mixed-sources", {
    caption: "Pixel 5 Add to workspace drawer with a local repository and folder configured",
  });

  const footer = drawer.locator("div.border-t").last();
  const [footerBox, submitBox] = await Promise.all([footer.boundingBox(), submit.boundingBox()]);
  expect(footerBox).not.toBeNull();
  expect(submitBox).not.toBeNull();
  expect(submitBox!.y + submitBox!.height).toBeLessThanOrEqual(drawerBox!.y + drawerBox!.height);
  await submit.tap();
  await expect(drawer).not.toBeVisible();
  await expect(entryPoint).toBeEnabled();
  await expect(entryPoint).toBeFocused();

  const files = testPage;
  await expect(
    files.getByTestId("file-tree-node").filter({ hasText: "mobile-local-repository-main" }),
  ).toBeVisible({ timeout: 30_000 });
  await expect(
    files.getByTestId("file-tree-node").filter({ hasText: "mobile-local-folder" }),
  ).toBeVisible();
  await testPage.reload();
  await session.waitForLoad();
  await testPage.getByRole("button", { name: "Files" }).tap();
  await expect(
    files.getByTestId("file-tree-node").filter({ hasText: "mobile-local-repository-main" }),
  ).toBeVisible({ timeout: 30_000 });
  await expect(
    files.getByTestId("file-tree-node").filter({ hasText: "mobile-local-folder" }),
  ).toBeVisible();
  await expect(testPage.locator("html")).toHaveJSProperty(
    "scrollWidth",
    await testPage.evaluate(() => innerWidth),
  );

  // The mobile chat surface must use the task workspace root when opening an
  // absolute link into the attached repository.
  const taskState = await apiClient.getTask(task.id);
  const activeSessionId = taskState.primary_session_id;
  expect(activeSessionId).toBeTruthy();
  const sessionData = await apiClient.listTaskSessions(task.id);
  const linkedRepoPath = sessionData.sessions
    .flatMap((item) => item.worktrees ?? [])
    .map((worktree) => worktree.worktree_path)
    .find((worktreePath) => worktreePath?.endsWith("mobile-local-repository-main"));
  expect(linkedRepoPath).toBeTruthy();
  await apiClient.seedSessionMessage(activeSessionId!, {
    type: "message",
    content: `[mobile source](${path.join(linkedRepoPath!, "mobile-repository.txt")})`,
  });

  await testPage.reload();
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  await testPage.getByRole("button", { name: "Chat" }).tap();
  const chatLink = session.activeChat().getByRole("link", { name: "mobile source" });
  await expect(chatLink).toBeVisible({ timeout: 15_000 });
  await chatLink.tap();
  const viewer = testPage.getByTestId("mobile-file-viewer-panel");
  await expect(viewer).toBeVisible({ timeout: 15_000 });
  await expect(viewer.locator(".cm-line").filter({ hasText: "repository source" })).toBeVisible();
  await expect(viewer.getByRole("button", { name: "Close" })).toBeVisible();
  await expect(
    viewer.getByText("mobile-local-repository-main/mobile-repository.txt"),
  ).toBeVisible();
  await expect(testPage.locator("html")).toHaveJSProperty(
    "scrollWidth",
    await testPage.evaluate(() => innerWidth),
  );
});
