// AC-COORDINATOR-PERMISSIONS-002.1, -002.2
// (docs/specs/coordinator/system-design/permissions.md#registration): a tool
// whose action the stored policy denies is not registered for the
// conversation, so the mock agent's call to it fails inside the MCP server,
// never reaches the backend guard, creates no proposal and writes no refused
// row.
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { waitForSessionState } from "../../helpers/session";
import { linkToCoordinatorNeedsYou } from "../../../lib/coordinator/links";

const POLICY_DENYING_CREATE_TASK = {
  version: 1,
  actions: {
    create_task: "denied",
    start_agent: "denied",
    message: "denied",
    move: "denied",
    resume: "denied",
    stop: "denied",
  },
};

test.describe("Coordinator policy enforcement", () => {
  test("Create a task Denied leaves propose_task_kandev unregistered for the conversation", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const release = await backend.useEnv({
      KANDEV_FEATURES_COORDINATOR: "true",
      KANDEV_FEATURES_COORDINATOR_PHASE2: "true",
    });
    try {
      const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
        name: "Policy Coordinator",
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: seedData.worktreeExecutorProfileId,
        task_agent_profile_id: seedData.agentProfileId,
        task_executor_profile_id: seedData.worktreeExecutorProfileId,
      });
      const workspacePath = `/api/v1/workspaces/${seedData.workspaceId}/coordinators/${coordinator.id}`;
      const saved = await apiClient.rawRequest("PUT", `${workspacePath}/settings`, {
        policy: POLICY_DENYING_CREATE_TASK,
      });
      expect(saved.status).toBe(200);

      await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
      const conversationOpened = waitForHttp(
        testPage,
        "POST",
        /\/coordinators\/[^/]+\/conversation$/,
      );
      await testPage.getByTestId("coordinator-copilot-launcher").click();
      const opened = (await (await conversationOpened).json()) as {
        task_id: string;
        session_id: string;
      };
      const popover = testPage.getByTestId("coordinator-copilot-popover");
      const editor = popover.getByTestId("chat-input-editor");
      await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 15_000 });

      const args = {
        title: "Denied by policy",
        description: "Should never be stored.",
        rationale: "The stored policy denies Create a task.",
        workflow_id: seedData.workflowId,
      };
      await editor.fill(`e2e:mcp:kandev:propose_task_kandev(${JSON.stringify(args)})`);
      await editor.press(`${process.platform === "darwin" ? "Meta" : "Control"}+Enter`);

      // The scripted call finishes (the agent turn ends) without a proposal card.
      await waitForSessionState(apiClient, {
        taskId: opened.task_id,
        sessionId: opened.session_id,
        expectedState: "WAITING_FOR_INPUT",
        message: "the mock agent's turn ends after the failed tool call",
        timeout: 30_000,
      });
      await expect(popover.locator('[data-testid^="proposal-card-"]')).toHaveCount(0);

      const proposals = await apiClient.rawRequest("GET", `${workspacePath}/proposals`);
      const body = (await proposals.json()) as { proposals: unknown[] };
      expect(body.proposals).toHaveLength(0);

      const activity = await apiClient.rawRequest("GET", `${workspacePath}/activity`);
      if (activity.ok) {
        const log = (await activity.json()) as { entries?: Array<{ outcome?: string }> };
        expect((log.entries ?? []).filter((e) => e.outcome === "refused")).toHaveLength(0);
      }
    } finally {
      await release();
    }
  });
});
