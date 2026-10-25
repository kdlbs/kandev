import fs from "node:fs";
import path from "node:path";
import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { GitHelper, makeGitEnv, createStandardProfile } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";

// Hold only the real save receipt; disk writes, reads, and workspace events stay live.
async function holdSaveReceipt(page: Page, filePath: string) {
  let requestId: string | undefined;
  let held: string | undefined;
  let release = () => {};
  await page.routeWebSocket(/\/ws$/, (socket) => {
    const server = socket.connectToServer();
    socket.onMessage((message) => {
      if (typeof message === "string") {
        for (const part of message.split("\n").filter(Boolean)) {
          const frame = JSON.parse(part);
          if (
            frame.type === "request" &&
            frame.action === "workspace.file.update" &&
            frame.payload?.path === filePath
          ) {
            requestId = frame.id;
          }
        }
      }
      server.send(message);
    });
    server.onMessage((message) => {
      if (typeof message !== "string") {
        socket.send(message);
        return;
      }
      const pass: string[] = [];
      for (const part of message.split("\n").filter(Boolean)) {
        const frame = JSON.parse(part);
        if (requestId && frame.type === "response" && frame.id === requestId) {
          expect(frame.payload).toMatchObject({ success: true });
          held = part;
          release = () => {
            if (!held) return;
            socket.send(part);
            requestId = undefined;
            held = undefined;
          };
        } else pass.push(part);
      }
      if (pass.length) socket.send(pass.join("\n"));
    });
  });
  return { held: () => Boolean(held), release: () => release() };
}

async function replaceEditorText(page: Page, content: string) {
  await page.locator(".monaco-editor:visible").click();
  await page.keyboard.press("ControlOrMeta+a");
  await page.keyboard.type(content);
}

async function publishAgentEdit(
  page: Page,
  session: SessionPage,
  diskPath: string,
  filePath: string,
  content: string,
) {
  fs.writeFileSync(diskPath, content);
  await session.clickTab("Files");
  const tab = page.locator(".dv-default-tab").filter({ hasText: filePath });
  await expect(tab).toHaveCount(1);
  await tab.click();
}

async function assertAcceptedBaselineSurvivesWarning(
  page: Page,
  session: SessionPage,
  diskPath: string,
  filePath: string,
  hold: Awaited<ReturnType<typeof holdSaveReceipt>>,
) {
  const accepted = "accepted baseline content";
  await replaceEditorText(page, "second submitted save");
  await page.keyboard.press("ControlOrMeta+s");
  await expect.poll(hold.held).toBe(true);
  await replaceEditorText(page, "typing during second save");
  await publishAgentEdit(page, session, diskPath, filePath, accepted);
  const reload = page.getByRole("button", { name: "Reload", exact: true });
  await expect(reload).toHaveCount(1);
  await reload.click();
  await expect(page.locator(".monaco-editor:visible .view-lines")).toContainText(accepted);
  await replaceEditorText(page, "typing after accepted baseline");
  await publishAgentEdit(page, session, diskPath, filePath, "latest agent content\n");
  await expect(reload).toBeVisible();
  hold.release();
  const save = page.getByRole("button", { name: /^Save \(/ });
  await expect(save).toBeEnabled();
  await expect(reload).toBeVisible();
  await replaceEditorText(page, accepted);
  await expect(save).toBeDisabled();
  expect(fs.readFileSync(diskPath, "utf8")).toBe("latest agent content\n");
}

test("keeps Reload available for an agent edit observed before a delayed save receipt", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  // @covers AC-UI-FILE-EDITOR-MUTATION-001.10
  test.setTimeout(90_000);
  const filePath = `save-refresh-${Date.now()}.txt`;
  const git = new GitHelper(seedData.repositoryPath, makeGitEnv(backend.tmpDir));
  const seedBaseline = git.getCurrentSha();
  const seedStatus = git.exec("git status --porcelain");
  const hold = await holdSaveReceipt(testPage, filePath);
  const profile = await createStandardProfile(apiClient, `save-refresh-${Date.now()}`);
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Save refresh",
    profile.id,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      executor_profile_id: seedData.worktreeExecutorProfileId,
    },
  );
  let diskPath = "";
  try {
    await expect
      .poll(
        async () => {
          const environment = await apiClient.getTaskEnvironment(task.id);
          const root = environment?.repos?.[0]?.worktree_path ?? environment?.workspace_path;
          diskPath = root ? path.join(root, filePath) : "";
          return Boolean(
            environment?.status === "ready" && diskPath && fs.existsSync(path.dirname(diskPath)),
          );
        },
        { timeout: 30_000 },
      )
      .toBe(true);
    expect(fs.realpathSync(path.dirname(diskPath))).not.toBe(
      fs.realpathSync(seedData.repositoryPath),
    );
    fs.writeFileSync(diskPath, "initial disk content\n");
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle();
    await session.clickTab("Files");
    const file = await session.fileTree.waitForFileTreeNode(filePath);
    await file.click();
    const editor = testPage.locator(".monaco-editor:visible");
    await expect(editor).toHaveCount(1);
    await expect(editor.locator(".view-lines")).toContainText("initial disk content");
    await editor.click();
    await testPage.keyboard.press("ControlOrMeta+a");
    await testPage.keyboard.type("user saved content");
    await testPage.keyboard.press("ControlOrMeta+s");
    await expect.poll(hold.held).toBe(true);
    expect(fs.readFileSync(diskPath, "utf8")).toBe("user saved content");
    await editor.click();
    await testPage.keyboard.press("ControlOrMeta+End");
    await testPage.keyboard.type(" plus later typing");
    fs.writeFileSync(diskPath, "newer agent content\n");
    // Activating the existing editor runs its actual workspace-sync producer.
    await session.clickTab("Files");
    const tab = testPage.locator(".dv-default-tab").filter({ hasText: filePath });
    await expect(tab).toHaveCount(1);
    await tab.click();
    const reload = testPage.getByRole("button", { name: "Reload", exact: true });
    await expect(reload).toHaveCount(1);
    await expect(reload).toBeVisible();
    hold.release();
    await expect(testPage.getByRole("button", { name: /^Save \(/ })).toBeEnabled();
    await expect(reload).toBeVisible();
    await expect(editor.locator(".view-lines")).toContainText(
      "user saved content plus later typing",
    );
    await reload.click();
    await expect(editor.locator(".view-lines")).toContainText("newer agent content");
    await expect(reload).toHaveCount(0);
    expect(fs.readFileSync(diskPath, "utf8")).toBe("newer agent content\n");
    await assertAcceptedBaselineSurvivesWarning(testPage, session, diskPath, filePath, hold);
  } finally {
    hold.release();
    if (diskPath) fs.rmSync(diskPath, { force: true });
    expect(git.getCurrentSha()).toBe(seedBaseline);
    expect(git.exec("git status --porcelain")).toBe(seedStatus);
  }
});
