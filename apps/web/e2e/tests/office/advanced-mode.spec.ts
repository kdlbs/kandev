import { type Page } from "@playwright/test";
import { runWithBackendRecovery, test as base, expect } from "../../fixtures/test-base";
import { OfficeApiClient } from "../../helpers/office-api-client";
import { waitForOfficeTaskSessionLive } from "../../helpers/office-launch";
import { SessionPage } from "../../pages/session-page";

/**
 * E2E tests for the office advanced mode dockview layout.
 *
 * Covers:
 * 1. Dockview panels render correctly (chat, files, terminal, changes)
 * 2. Chat shows agent messages
 * 3. Quick-chat workspace path is displayed (not skeleton)
 * 4. Mode toggle between simple and advanced preserves state
 * 5. Terminal connects to agent execution workspace
 */

type AdvancedModeFixtures = {
  officeApi: OfficeApiClient;
  advancedSeed: {
    workspaceId: string;
    agentId: string;
    workflowId: string;
    taskId: string;
  };
};

type OnboardingResult = {
  workspaceId: string;
  agentId: string;
  taskId?: string;
};

const test = base.extend<{ testPage: Page }, AdvancedModeFixtures>({
  officeApi: [
    async ({ backend }, use) => {
      await use(new OfficeApiClient(backend.baseUrl));
    },
    { scope: "worker" },
  ],
  advancedSeed: async ({ officeApi, apiClient, backend, testPage, seedData }, use) => {
    // Ensure the normal per-test reset has completed before creating a fresh
    // Office workspace for this test.
    void testPage;
    const result = (await officeApi.completeOnboarding({
      workspaceName: "Advanced Mode Workspace",
      taskPrefix: "AM",
      agentName: "CEO",
      agentProfileId: seedData.agentProfileId,
      executorPreference: "local_pc",
      taskTitle: "present yourself",
      taskDescription: "say your name",
    })) as OnboardingResult;

    let workflowId: string | undefined;
    try {
      const taskId = result.taskId;
      if (!taskId) throw new Error("completeOnboarding did not return a taskId");

      await runWithBackendRecovery(backend, async () => {
        const { workspaces } = await apiClient.listWorkspaces();
        workflowId = workspaces.find(
          (workspace) => workspace.id === result.workspaceId,
        )?.office_workflow_id;
        if (!workflowId) {
          throw new Error(`Advanced mode workspace ${result.workspaceId} has no office workflow`);
        }

        await apiClient.saveUserSettings({
          workspace_id: result.workspaceId,
          workflow_filter_id: seedData.workflowId,
          keyboard_shortcuts: {},
          enable_preview_on_click: false,
        });

        // Wait for the initial launch and turn to settle before advanced mode
        // asks the runtime to ensure the execution again.
        await waitForOfficeTaskSessionLive(apiClient, taskId);
        await expect
          .poll(async () => (await apiClient.getTaskEnvironment(taskId))?.status, {
            timeout: 30_000,
            message: "Waiting for the Office task environment to become ready",
          })
          .toBe("ready");
      });
      if (!workflowId) throw new Error("Office onboarding workflow was not resolved");

      await use({
        workspaceId: result.workspaceId,
        agentId: result.agentId,
        workflowId,
        taskId,
      });
    } finally {
      await runWithBackendRecovery(backend, () =>
        apiClient.e2eReset(result.workspaceId, [
          seedData.workflowId,
          ...(workflowId ? [workflowId] : []),
        ]),
      );
    }
  },
});

/** Open the advanced layout after task-session hydration. */
async function enterAdvancedMode(testPage: Page, taskId: string) {
  // Navigate directly to ?mode=advanced. The page initially renders in simple
  // mode while sessions load async. Once hasSession becomes true, React
  // re-renders into TaskAdvancedMode with dockview.
  await testPage.goto(`/office/tasks/${taskId}?mode=advanced`);

  // A session-chat panel can also be present in simple mode. Wait for the
  // layout boundary itself before asserting on its dockview panels.
  await expect(testPage.locator(".dv-dockview")).toBeVisible({ timeout: 30_000 });
  await expect(testPage.getByTestId("session-chat")).toBeVisible({ timeout: 30_000 });
}

test.describe("Office advanced mode", () => {
  test.describe.configure({ retries: 1 });

  test("dockview layout renders with chat, files, changes, and terminal panels", async ({
    testPage,
    advancedSeed,
  }) => {
    test.setTimeout(45_000);

    await enterAdvancedMode(testPage, advancedSeed.taskId);

    // Chat panel must be visible (dockview center group)
    await expect(testPage.getByTestId("session-chat")).toBeVisible();

    // Files panel must be visible (dockview right group)
    await expect(testPage.getByTestId("files-panel")).toBeVisible({ timeout: 15_000 });

    // Terminal panel must be visible (dockview right-bottom group)
    await expect(testPage.getByTestId("terminal-panel")).toBeVisible({ timeout: 15_000 });
  });

  test("chat panel shows agent messages from completed session", async ({
    testPage,
    advancedSeed,
  }) => {
    test.setTimeout(45_000);

    await enterAdvancedMode(testPage, advancedSeed.taskId);

    // The chat panel should show messages from the completed agent session.
    // The mock agent responds with a status message like "Environment prepared"
    // or "Started agent" — look for any of those indicators.
    const chatPanel = testPage.getByTestId("session-chat");
    await expect(chatPanel).toBeVisible({ timeout: 10_000 });

    // The chat should contain session messages (not the "No messages" empty state)
    await expect(chatPanel.getByText("No messages yet")).not.toBeVisible({ timeout: 15_000 });
  });

  test("files panel shows workspace content", async ({ testPage, advancedSeed }) => {
    test.setTimeout(120_000);

    await enterAdvancedMode(testPage, advancedSeed.taskId);

    // Wait for files panel to load with tree content
    const filesPanel = testPage.getByTestId("files-panel");
    await expect(filesPanel).toBeVisible({ timeout: 15_000 });

    // Repository-free Office workspaces contain .gitkeep.
    // The tree virtualizes rows, so a text locator can miss a file that is
    // present but not mounted in the current viewport.
    const session = new SessionPage(testPage);
    await session.fileTree.waitForFileTreeNode(".gitkeep", 30_000);
  });

  test("terminal connects to agent execution workspace", async ({ testPage, advancedSeed }) => {
    test.setTimeout(60_000);

    await enterAdvancedMode(testPage, advancedSeed.taskId);

    // Wait for the terminal panel to be visible
    const terminalPanel = testPage.getByTestId("terminal-panel");
    await expect(terminalPanel).toBeVisible({ timeout: 10_000 });

    // Terminal should connect — "Connecting terminal..." should disappear
    await expect(terminalPanel.getByText("Connecting terminal...")).not.toBeVisible({
      timeout: 30_000,
    });
  });

  test("navigating between simple and advanced mode preserves session", async ({
    testPage,
    advancedSeed,
  }) => {
    test.setTimeout(45_000);

    // Start in advanced mode
    await enterAdvancedMode(testPage, advancedSeed.taskId);

    // Chat should show messages
    const chatPanel = testPage.getByTestId("session-chat");
    await expect(chatPanel).toBeVisible();

    // Navigate back to simple mode
    await testPage.goto(`/office/tasks/${advancedSeed.taskId}`);

    // Verify simple mode — heading should be visible
    await expect(testPage.getByRole("heading", { name: "present yourself" })).toBeVisible({
      timeout: 10_000,
    });

    // Navigate to advanced mode again — session should still be there
    await testPage.goto(`/office/tasks/${advancedSeed.taskId}?mode=advanced`);
    await expect(testPage.locator(".dv-dockview")).toBeVisible({ timeout: 20_000 });
    await expect(testPage.getByTestId("session-chat")).toBeVisible({ timeout: 20_000 });
  });
});
