import { expect, test } from "../../fixtures/test-base";
import {
  agentWorktreeGroup,
  agentWorktreePrompt,
  AGENT_WORKTREE_FILE,
  expectAgentWorktreeGroup,
  removeAgentWorktreeRoot,
} from "../../helpers/agent-worktree-adoption";
import { waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

test.describe("Agent worktree adoption", () => {
  test.setTimeout(240_000);

  test("detects, removes, and re-attaches an agent-created worktree", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Agent worktree adoption",
      seedData.agentProfileId,
      {
        description: agentWorktreePrompt(false),
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repositories: [{ repository_id: seedData.repositoryId, base_branch: "main" }],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    if (!task.session_id) throw new Error("task creation did not return a session id");
    await waitForSessionDone(apiClient, task.id, task.session_id, "initial turn", 120_000);

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 60_000 });
    await session.clickTab("Changes");

    const group = await expectAgentWorktreeGroup(testPage);
    await expect(group.getByText(AGENT_WORKTREE_FILE, { exact: true })).toBeVisible({
      timeout: 30_000,
    });

    await removeAgentWorktreeRoot(testPage);
    expect(await agentWorktreeGroup(testPage).count()).toBe(0);

    await apiClient.addUserMessage(task.id, task.session_id, agentWorktreePrompt(true));
    await waitForSessionDone(apiClient, task.id, task.session_id, "reported turn", 120_000);
    const reattached = await expectAgentWorktreeGroup(testPage);
    await expect(reattached.getByText(AGENT_WORKTREE_FILE, { exact: true })).toBeVisible({
      timeout: 30_000,
    });
  });
});
