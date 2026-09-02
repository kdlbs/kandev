import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import {
  GitHelper,
  makeGitEnv,
  openTaskSession,
  createStandardProfile,
} from "../../helpers/git-helper";

/**
 * End-to-end coverage for workspace file transfer.
 *
 * Drag and drop from the operating system is deliberately absent: it is out of
 * scope for this capability, and Playwright cannot drive an OS-level drag
 * reliably anyway. A test that appeared to cover it would be worse than none.
 */

const UPLOAD_INPUT = '[data-testid="files-upload-input"]';
const FOLDER_INPUT = '[data-testid="files-upload-folder-input"]';
const UPLOAD_ROUTE = /\/api\/v1\/task-sessions\/[^/]+\/workspace\/files$/;
const PREFLIGHT_ROUTE = /\/api\/v1\/task-sessions\/[^/]+\/workspace\/files\/preflight$/;

async function openFilesPanel(
  testPage: Page,
  apiClient: ApiClient,
  seedData: { workspaceId: string; workflowId: string; startStepId: string; repositoryId: string },
  profileName: string,
  taskTitle: string,
) {
  const profile = await createStandardProfile(apiClient, profileName);
  await apiClient.createTaskWithAgent(seedData.workspaceId, taskTitle, profile.id, {
    description: "/e2e:simple-message",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  const session = await openTaskSession(testPage, taskTitle);
  await session.clickTab("Files");
  return session;
}

function seedRepo(backend: { tmpDir: string }, files: Record<string, string | Buffer>) {
  const git = new GitHelper(
    path.join(backend.tmpDir, "repos", "e2e-repo"),
    makeGitEnv(backend.tmpDir),
  );
  for (const [name, content] of Object.entries(files)) git.createFile(name, content);
  git.stageAll();
  git.commit("seed");
}

test.describe("Workspace file transfer", () => {
  test("uploads picked files and shows them in the tree", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    seedRepo(backend, { "existing.ts": "seed" });

    const session = await openFilesPanel(
      testPage,
      apiClient,
      seedData,
      "wft-upload-1",
      "WFT Upload Files",
    );
    await expect(session.fileTreeNode("existing.ts")).toBeVisible({ timeout: 15_000 });

    const uploaded = waitForHttp(testPage, "POST", UPLOAD_ROUTE, {
      predicate: (response) => response.status() === 201,
    });
    await testPage.setInputFiles(UPLOAD_INPUT, {
      name: "fixture.json",
      mimeType: "application/json",
      buffer: Buffer.from('{"ok":true}'),
    });
    await uploaded;

    await expect(session.fileTreeNode("fixture.json")).toBeVisible();
  });

  test("prompts on a name collision and keeps both copies", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    seedRepo(backend, { "taken.txt": "original" });

    const session = await openFilesPanel(
      testPage,
      apiClient,
      seedData,
      "wft-conflict-1",
      "WFT Upload Conflict",
    );
    await expect(session.fileTreeNode("taken.txt")).toBeVisible({ timeout: 15_000 });

    const preflighted = waitForHttp(testPage, "POST", PREFLIGHT_ROUTE);
    await testPage.setInputFiles(UPLOAD_INPUT, {
      name: "taken.txt",
      mimeType: "text/plain",
      buffer: Buffer.from("incoming"),
    });
    await preflighted;

    // The dialog appears before anything is written.
    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText("taken.txt")).toBeVisible();

    const uploaded = waitForHttp(testPage, "POST", UPLOAD_ROUTE, {
      predicate: (response) => response.status() === 201,
    });
    await dialog.getByRole("button", { name: "Upload" }).click();
    await uploaded;

    // Keep both is the default, so the original survives beside the new copy.
    await expect(session.fileTreeNode("taken-1.txt")).toBeVisible();
    await expect(session.fileTreeNode("taken.txt")).toBeVisible();
  });

  test("cancelling the conflict prompt writes nothing at all", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    seedRepo(backend, { "taken.txt": "original" });

    const session = await openFilesPanel(
      testPage,
      apiClient,
      seedData,
      "wft-cancel-1",
      "WFT Upload Cancel",
    );
    await expect(session.fileTreeNode("taken.txt")).toBeVisible({ timeout: 15_000 });

    let uploadAttempted = false;
    testPage.on("request", (request) => {
      if (request.method() === "POST" && UPLOAD_ROUTE.test(new URL(request.url()).pathname)) {
        uploadAttempted = true;
      }
    });

    const preflighted = waitForHttp(testPage, "POST", PREFLIGHT_ROUTE);
    await testPage.setInputFiles(UPLOAD_INPUT, [
      { name: "taken.txt", mimeType: "text/plain", buffer: Buffer.from("incoming") },
      { name: "fresh.txt", mimeType: "text/plain", buffer: Buffer.from("new") },
    ]);
    await preflighted;

    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await dialog.getByRole("button", { name: "Cancel" }).click();
    await expect(dialog).toBeHidden();

    // The assertion that matters is an absence: the unconflicted file in the
    // same selection must not have been written either.
    expect(uploadAttempted).toBe(false);
    await expect(session.fileTreeNode("fresh.txt")).toHaveCount(0);
  });

  test("uploads a folder and recreates its structure", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    seedRepo(backend, { "existing.ts": "seed" });

    const session = await openFilesPanel(
      testPage,
      apiClient,
      seedData,
      "wft-folder-1",
      "WFT Upload Folder",
    );
    await expect(session.fileTreeNode("existing.ts")).toBeVisible({ timeout: 15_000 });

    const uploaded = waitForHttp(testPage, "POST", UPLOAD_ROUTE, {
      predicate: (response) => response.status() === 201,
    });
    // A webkitdirectory input takes a directory path, not file descriptors:
    // Playwright derives each webkitRelativePath from the tree on disk.
    const bundleDir = path.join(backend.tmpDir, "upload-bundle");
    fs.mkdirSync(path.join(bundleDir, "nested"), { recursive: true });
    fs.writeFileSync(path.join(bundleDir, "nested", "leaf.txt"), "leaf");
    await testPage.setInputFiles(FOLDER_INPUT, bundleDir);
    await uploaded;

    await expect(session.fileTreeNode("upload-bundle")).toBeVisible({ timeout: 15_000 });
  });

  test("downloads an unpreviewable file from its viewer", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    seedRepo(backend, {
      "archive.zip": Buffer.from([0x50, 0x4b, 0x03, 0x04, 0x00, 0x01, 0x02, 0xff, 0xfe]),
    });

    const session = await openFilesPanel(
      testPage,
      apiClient,
      seedData,
      "wft-download-1",
      "WFT Download Binary",
    );

    const node = session.fileTreeNode("archive.zip");
    await expect(node).toBeVisible({ timeout: 15_000 });
    await node.click();

    const downloadControl = testPage.getByRole("button", { name: "Download file" });
    await expect(downloadControl).toBeVisible();

    const download = testPage.waitForEvent("download");
    await downloadControl.click();
    const saved = await download;
    expect(saved.suggestedFilename()).toBe("archive.zip");
  });
});
