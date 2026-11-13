import { expect, test } from "../../fixtures/test-base";
import {
  agentWorktreePrompt,
  AGENT_WORKTREE_FILE,
  expectAgentWorktreeAttached,
  expectAgentWorktreeGroup,
  removeAgentWorktreeRoot,
  waitForAgentWorktreeStatus,
  waitForAgentWorktreeTaskUpdate,
} from "../../helpers/agent-worktree-adoption";
import { watchWs } from "../../helpers/causal-waits";
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

    // Detection runs from the live session poll, so the page has to be open.
    // The status event is the git snapshot that renders the adopted file list.
    const watcher = watchWs(testPage);
    const statusPublished = waitForAgentWorktreeStatus(watcher, task.session_id);
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle();
    await session.clickTab("Changes");
    await expectAgentWorktreeAttached(apiClient, task.id);
    await statusPublished;

    const group = await expectAgentWorktreeGroup(testPage);
    await expect(group.getByText(AGENT_WORKTREE_FILE, { exact: true })).toBeVisible();

    await removeAgentWorktreeRoot(testPage);

    const reattachedTask = waitForAgentWorktreeTaskUpdate(watcher, task.id);
    const reattachedStatus = waitForAgentWorktreeStatus(watcher, task.session_id);
    await apiClient.addUserMessage(task.id, task.session_id, agentWorktreePrompt(true));
    await waitForSessionDone(apiClient, task.id, task.session_id, "reported turn", 120_000);
    await reattachedTask;
    await reattachedStatus;

    const reattached = await expectAgentWorktreeGroup(testPage);
    await expect(reattached.getByText(AGENT_WORKTREE_FILE, { exact: true })).toBeVisible();
  });
});
