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

test.describe("VS Code open panel", () => {
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
   * Regression test for: "dockview: referencePanel 'chat' does not exist"
   *
   * When opening the embedded VS Code panel via the top-bar editor button,
   * the dockview layout must not throw if the 'chat' reference panel is absent.
   */
  test("opens VS Code panel via Open Editor without referencePanel error", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Track page errors to catch the dockview referencePanel exception
    const pageErrors: Error[] = [];
    testPage.on("pageerror", (err) => pageErrors.push(err));

    const { session } = await seedTaskWithSession(
      testPage,
      apiClient,
      seedData,
      "VSCode Open Panel Test",
    );

    // Click the main "Open editor" button (the IconCode button in EditorsMenu).
    // The tooltip reads "Open editor", so target the button inside that wrapper.
    const editorBtn = testPage.locator("button:has(.tabler-icon-code)").first();
    await expect(editorBtn).toBeEnabled({ timeout: 10_000 });
    await editorBtn.click();

    // Assert: VS Code tab appears in dockview
    await expect(session.vscodeTab()).toBeVisible({ timeout: 10_000 });

    // Assert: "Starting VS Code Server" progress text is visible while booting
    await expect(testPage.getByText("Starting VS Code Server")).toBeVisible({ timeout: 30_000 });

    // Assert: code-server iframe loads
    await expect(session.vscodeIframe()).toBeVisible({ timeout: 90_000 });

    // Assert: no dockview referencePanel errors were thrown
    const referencePanelErrors = pageErrors.filter(
      (e) => e.message.includes("referencePanel") && e.message.includes("does not exist"),
    );
    expect(referencePanelErrors).toHaveLength(0);
  });
});
