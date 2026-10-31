import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { createStandardProfile, GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { watchWs } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";

test.use({ trace: "retain-on-failure" });

// @covers AC-UI-FILE-EDITOR-MUTATION-002.1
// An editor open before a Files rename must save its retained dirty content to
// the renamed file. The first save proves the editor and fixture are writable.
test("saves an open dirty editor after renaming its file in Files", async ({
  testPage,
  apiClient,
  seedData,
  backend,
  prCapture,
}) => {
  const original = "continuity-original.txt";
  const renamed = "continuity-renamed.txt";
  const control = "saved before rename";
  const edited = "saved after rename";
  const git = new GitHelper(seedData.repositoryPath, makeGitEnv(backend.tmpDir));
  const initialHead = git.getCurrentSha();
  const initialStatus = git.exec("git status --porcelain");
  const profile = await createStandardProfile(apiClient, `editor-rename-${Date.now()}`);
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Open editor rename continuity",
    profile.id,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      executor_profile_id: seedData.worktreeExecutorProfileId,
    },
  );
  let workspace = "";
  await expect
    .poll(
      async () => {
        const environment = await apiClient.getTaskEnvironment(task.id);
        workspace = environment?.repos?.[0]?.worktree_path ?? environment?.workspace_path ?? "";
        return Boolean(environment?.status === "ready" && workspace && fs.existsSync(workspace));
      },
      { timeout: 30_000 },
    )
    .toBe(true);
  expect(fs.realpathSync(workspace)).not.toBe(fs.realpathSync(seedData.repositoryPath));
  const source = path.join(workspace, original);
  const destination = path.join(workspace, renamed);
  try {
    fs.writeFileSync(source, "original disk content\n");
    await testPage.addInitScript(() => {
      localStorage.setItem(
        "kandev-editor-providers",
        JSON.stringify({
          version: 3,
          state: {
            providers: {
              "code-editor": "monaco",
              "diff-viewer": "pierre-diffs",
              "chat-code-block": "shiki",
              "chat-diff": "pierre-diffs",
              "plan-editor": "tiptap",
            },
          },
        }),
      );
    });
    const ws = watchWs(testPage);
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.clickTab("Files");
    const file = await session.fileTree.waitForFileTreeNode(original);
    await file.click();
    const editor = testPage.locator(".monaco-editor:visible");
    await expect(editor).toHaveCount(1);
    await expect(editor.locator(".view-lines")).toContainText("original disk content");
    await editor.click();
    await testPage.keyboard.press("ControlOrMeta+A");
    await testPage.keyboard.type(control);
    const firstSave = ws.waitForResponse("workspace.file.update", {
      requestWhere: (payload) => payload.path === original,
    });
    await testPage.getByRole("button", { name: /^Save \(/ }).click();
    expect((await firstSave).payload).toMatchObject({ success: true });
    expect(fs.readFileSync(source, "utf8")).toBe(control);
    await editor.click();
    await testPage.keyboard.press("ControlOrMeta+A");
    await testPage.keyboard.type(edited);
    await expect(testPage.getByRole("button", { name: /^Save \(/ })).toBeEnabled();

    await file.click({ button: "right" });
    await testPage.getByRole("menuitem", { name: "Rename" }).click();
    const name = file.getByRole("textbox");
    await expect(name).toBeVisible();
    await name.fill(renamed);
    const rename = ws.waitForResponse("workspace.file.rename", {
      requestWhere: (payload) => payload.old_path === original && payload.new_path === renamed,
    });
    await name.press("Enter");
    expect((await rename).payload).toMatchObject({ success: true });
    expect(fs.existsSync(source)).toBe(false);
    expect(fs.readFileSync(destination, "utf8")).toBe(control);
    await expect(editor.locator(".view-lines")).toContainText(edited);
    const finalSave = ws.waitForResponse("workspace.file.update", {
      requestWhere: (payload) => payload.path === renamed && payload.session_id === task.session_id,
      timeout: 5_000,
    });
    await testPage.getByRole("button", { name: /^Save \(/ }).click();
    expect((await finalSave).payload).toMatchObject({ success: true });
    expect(fs.readFileSync(destination, "utf8")).toBe(edited);
    expect(fs.existsSync(source)).toBe(false);
    await expect(testPage.locator(".dv-default-tab").filter({ hasText: renamed })).toHaveCount(1);
    await prCapture.screenshot("desktop-renamed-file", {
      caption: "Desktop editor after a file rename",
    });
    await (await session.fileTree.waitForFileTreeNode(renamed)).click();
    await expect(testPage.locator(".dv-default-tab").filter({ hasText: renamed })).toHaveCount(1);
  } finally {
    fs.rmSync(source, { force: true });
    fs.rmSync(destination, { force: true });
    expect(git.getCurrentSha()).toBe(initialHead);
    expect(git.exec("git status --porcelain")).toBe(initialStatus);
  }
});
