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
    await picker.getByTestId("folder-picker-entry").filter({ hasText: segment }).tap();
  }
  await picker.getByTestId("folder-picker-choose").tap();
}

test("mobile Files drawer attaches sources with fixed controls and persisted workspace", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(120_000);
  const gitEnv = makeGitEnv(backend.tmpDir);
  const repositoryPath = path.join(backend.tmpDir, "mobile-sources", "mobile-local-repository");
  const folderPath = path.join(backend.tmpDir, "mobile-sources", "mobile-local-folder");
  fs.mkdirSync(repositoryPath, { recursive: true });
  fs.mkdirSync(folderPath, { recursive: true });
  fs.writeFileSync(path.join(repositoryPath, "mobile-repository.txt"), "repository source\n");
  fs.writeFileSync(path.join(folderPath, "mobile-folder.txt"), "folder source\n");
  execFileSync("git", ["init", "-b", "main"], { cwd: repositoryPath, env: gitEnv });
  execFileSync("git", ["add", "."], { cwd: repositoryPath, env: gitEnv });
  execFileSync("git", ["commit", "-m", "initial source"], { cwd: repositoryPath, env: gitEnv });
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

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  await testPage.getByRole("button", { name: "Files" }).tap();
  const entryPoint = testPage.getByTestId("files-add-sources");
  await expect(entryPoint).toBeVisible();
  await expect(entryPoint).toBeEnabled();
  const entryBox = await entryPoint.boundingBox();
  expect(entryBox).not.toBeNull();
  expect(entryBox!.height).toBeGreaterThanOrEqual(44);
  await entryPoint.tap();

  const drawer = testPage.getByTestId("add-workspace-sources-drawer");
  await expect(drawer).toBeVisible();
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

  await drawer.getByRole("button", { name: "Local Git repository" }).tap();
  await drawer.getByRole("button", { name: "Local folder" }).tap();
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

  const footer = drawer.locator("div.border-t").last();
  const submit = drawer.getByTestId("add-workspace-sources-submit");
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
});
