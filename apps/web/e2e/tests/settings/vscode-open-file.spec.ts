import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

/**
 * Resolve the code-server install directory. Checks the default kandev install
 * path at ~/.kandev/tools/code-server/ (real HOME, not e2e temp HOME).
 */
function findCodeServerInstall(): string | null {
  const home = os.homedir();
  const installDir = path.join(home, ".kandev", "tools", "code-server");
  if (!fs.existsSync(installDir)) return null;

  const entries = fs.readdirSync(installDir);
  for (const entry of entries) {
    const binPath = path.join(installDir, entry, "bin", "code-server");
    if (fs.existsSync(binPath)) return installDir;
  }
  return null;
}

/**
 * Seed a task + session via the API and navigate directly to the session page.
 * Waits for the mock agent to complete its turn (idle input visible).
 */
async function seedTaskWithSession(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<{ session: SessionPage; sessionId: string }> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await testPage.goto(`/t/${task.id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

  return { session, sessionId: task.session_id };
}

test.describe("VS Code open file", () => {
  test.describe.configure({ retries: 1 });

  const codeServerDir = findCodeServerInstall();

  test.beforeEach(async ({ backend }) => {
    if (!codeServerDir) {
      test.skip(true, "code-server not installed — skipping VS Code e2e tests");
      return;
    }

    // Symlink the real code-server install into the e2e backend's HOME so
    // ResolveBinary finds the pre-installed binary without re-downloading.
    const targetDir = path.join(backend.tmpDir, ".kandev", "tools", "code-server");
    if (!fs.existsSync(targetDir)) {
      fs.mkdirSync(path.dirname(targetDir), { recursive: true });
      fs.symlinkSync(codeServerDir, targetDir);
    }
  });

  /**
   * Full UI-driven flow:
   *   1. Create a file in the workspace
   *   2. Click the file in the file tree → opens in built-in editor
   *   3. Click "Open with..." → "VS Code (Embedded)" in the editor toolbar
   *   4. Assert VS Code panel opens, code-server starts, and iframe loads
   */
  test("opens file in VS Code via Open With menu", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    // Create a file in the workspace repo BEFORE navigating so the file tree
    // picks it up on its initial load.
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const filePath = path.join(repoDir, "hello.txt");
    fs.writeFileSync(filePath, "Hello from e2e test!\n");

    const { session } = await seedTaskWithSession(
      testPage,
      apiClient,
      seedData,
      "VSCode Open File Test",
    );

    // Click the "Files" tab to ensure the file tree is visible
    await session.clickTab("Files");
    await expect(session.files).toBeVisible({ timeout: 5_000 });

    // Wait for the file tree to load and show our file
    const fileRow = session.files.getByText("hello.txt");
    await expect(fileRow).toBeVisible({ timeout: 10_000 });

    // Click the file → opens in the built-in Monaco editor panel
    await fileRow.click();

    // Wait for the file editor tab to appear in dockview
    const editorTab = testPage.locator(".dv-default-tab:has-text('hello.txt')");
    await expect(editorTab).toBeVisible({ timeout: 10_000 });

    // Click the "Open with..." button (IconExternalLink) in the Monaco editor toolbar.
    // Tabler icons render with class "tabler-icon tabler-icon-external-link".
    // Find the button that wraps this icon (the FileActionsDropdown trigger).
    const openWithBtn = testPage.locator("button:has(.tabler-icon-external-link)").first();
    await openWithBtn.waitFor({ state: "visible", timeout: 10_000 });
    await openWithBtn.click();

    // Click "VS Code (Embedded)" in the dropdown menu
    const vsCodeMenuItem = testPage.getByRole("menuitem", { name: /VS Code/ }).first();
    await expect(vsCodeMenuItem).toBeVisible({ timeout: 5_000 });
    await vsCodeMenuItem.click();

    // Assert: VS Code tab appears in dockview
    await expect(session.vscodeTab()).toBeVisible({ timeout: 10_000 });

    // Assert: "Starting VS Code Server" progress text is visible while booting
    await expect(testPage.getByText("Starting VS Code Server")).toBeVisible({ timeout: 30_000 });

    // Assert: code-server iframe loads (code-server is running)
    await expect(session.vscodeIframe()).toBeVisible({ timeout: 90_000 });

    // Assert: hello.txt is opened inside code-server (visible as a tab or in the editor).
    // The iframe is same-origin (served via kandev proxy) so frameLocator works.
    const vscodeFrame = testPage.frameLocator('iframe[title="VS Code"]');
    await expect(vscodeFrame.getByRole("tab", { name: /hello\.txt/ })).toBeVisible({
      timeout: 30_000,
    });
  });
});
