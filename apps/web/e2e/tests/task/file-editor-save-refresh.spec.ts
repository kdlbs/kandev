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
  git.createFile(filePath, "initial disk content\n");
  git.stageAll();
  git.commit("seed save refresh file");
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
          return Boolean(diskPath && fs.existsSync(diskPath));
        },
        { timeout: 30_000 },
      )
      .toBe(true);
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
  } finally {
    hold.release();
    fs.rmSync(path.join(seedData.repositoryPath, filePath), { force: true });
    if (diskPath) fs.rmSync(diskPath, { force: true });
  }
});
