import { expect, test } from "../../fixtures/test-base";
import {
  agentWorktreePrompt,
  confirmAgentWorktreeRemoval,
  expectAgentWorktreeAttached,
  expectAgentWorktreeGroup,
  waitForAgentWorktreeStatus,
} from "../../helpers/agent-worktree-adoption";
import { watchWs } from "../../helpers/causal-waits";
import { waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

async function openMobilePanel(page: import("@playwright/test").Page, name: RegExp) {
  await page.getByRole("navigation").getByRole("button", { name }).tap();
}

test.describe("Mobile agent worktree adoption", () => {
  test.setTimeout(240_000);

  test("shows a reported worktree and removes it through the root sheet", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile agent worktree adoption",
      seedData.agentProfileId,
      {
        description: agentWorktreePrompt(true),
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repositories: [{ repository_id: seedData.repositoryId, base_branch: "main" }],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    if (!task.session_id) throw new Error("task creation did not return a session id");
    await waitForSessionDone(apiClient, task.id, task.session_id, "reported turn", 120_000);
    await expectAgentWorktreeAttached(apiClient, task.id);

    const watcher = watchWs(testPage);
    const statusPublished = waitForAgentWorktreeStatus(watcher, task.session_id);
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle();
    await openMobilePanel(testPage, /Changes$/);
    await statusPublished;
    const group = await expectAgentWorktreeGroup(testPage);
    const header = group.getByTestId("changes-repo-header");

    // Radix's ContextMenu trigger listens for the native contextmenu event,
    // which is also what a touch long-press emits in the browser.
    await header.dispatchEvent("contextmenu", { bubbles: true, clientX: 40, clientY: 40 });
    await expect(testPage.getByTestId("root-context-menu")).toBeVisible();
    await testPage.getByTestId("root-menu-remove").click();
    await confirmAgentWorktreeRemoval(testPage);

    const overflow = await testPage.evaluate(() => {
      const root = document.documentElement;
      return root.scrollWidth - root.clientWidth;
    });
    expect(overflow).toBeLessThanOrEqual(1);
  });
});
