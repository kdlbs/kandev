// AC-COORDINATOR-COPILOT-006.1, .2, .7: the activity display on a phone viewport (mobile-chrome,
// full-screen panel)
// (docs/plans/workspace-coordinator/task-10-activity-display.md). One status line while a turn
// runs, one collapsed chip after it, start-up rows hidden once the agent booted.
import { type Locator, type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { linkToCoordinatorNeedsYou } from "../../../lib/coordinator/links";

const COORDINATOR_READ = /\/coordinators\/[^/]+$/;
const CONVERSATION_OPENED = /\/coordinators\/[^/]+\/conversation$/;

async function openCopilot(page: Page): Promise<Locator> {
  const coordinatorRead = waitForHttp(page, "GET", COORDINATOR_READ);
  const conversationOpened = waitForHttp(page, "POST", CONVERSATION_OPENED);
  const launcher = page.getByTestId("coordinator-copilot-launcher");
  await expect(launcher).toBeVisible({ timeout: 10_000 });
  await launcher.click();
  await coordinatorRead;
  await conversationOpened;
  const panel = page.getByTestId("coordinator-copilot-popover");
  await expect(panel).toBeVisible();
  return panel;
}

async function send(panel: Locator, text: string) {
  const editor = panel.getByTestId("chat-input-editor");
  await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 15_000 });
  await editor.fill(text);
  if (await panel.page().evaluate(() => window.matchMedia("(pointer: coarse)").matches)) {
    await panel.getByTestId("submit-message-button").tap();
    return;
  }
  await editor.press(`${process.platform === "darwin" ? "Meta" : "Control"}+Enter`);
}

test.describe("Coordinator copilot activity display on a phone viewport", () => {
  test("shows the status line, then one collapsed chip that expands (AC .006.1, .006.2, .006.4, .006.7)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Activity Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
      task_agent_profile_id: seedData.agentProfileId,
      task_executor_profile_id: seedData.worktreeExecutorProfileId,
    });
    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
    const panel = await openCopilot(testPage);

    await send(
      panel,
      [
        `e2e:mcp:kandev:list_workflows_kandev({"workspace_id":"${seedData.workspaceId}"})`,
        "e2e:delay(6000)",
        `e2e:mcp:kandev:list_tasks_kandev({"workflow_id":"${seedData.workflowId}"})`,
        'e2e:message("activity-done")',
      ].join("\n"),
    );

    const statusLine = panel.getByTestId("activity-status-line");
    await expect(statusLine).toBeVisible({ timeout: 30_000 });
    await expect(panel.getByText("Kandev: List Workflows")).toHaveCount(0);
    await expect(panel.getByTestId("activity-chip")).toHaveCount(0);

    await expect(panel.getByText("activity-done", { exact: true })).toBeVisible({
      timeout: 30_000,
    });
    await expect(statusLine).toHaveCount(0);
    const chip = panel.getByTestId("activity-chip").getByRole("button");
    await expect(panel.getByTestId("activity-chip")).toHaveCount(1);
    await expect(chip).toHaveText(/Checked 2 sources/);
    await expect(chip).toHaveAttribute("aria-expanded", "false");
    await expect(panel.getByText("Kandev: List Workflows")).toHaveCount(0);

    await chip.click();
    await expect(chip).toHaveAttribute("aria-expanded", "true");
    await expect(panel.getByText("Kandev: List Workflows")).toBeVisible();
    await expect(panel.getByText("Kandev: List Tasks")).toBeVisible();

    // Start-up rows are hidden once the agent booted.
    await expect(panel.getByText("Started agent", { exact: false })).toHaveCount(0);
    await expect(panel.getByText("Environment prepared", { exact: false })).toHaveCount(0);
  });
});
