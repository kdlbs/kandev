import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";

// @covers AC-AGENTS-AGENT-STALL-RECOVERY-001.6
test("a compaction stall notice clears on tool activity and stays cleared after reload", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const task = await apiClient.createTask(seedData.workspaceId, "Compaction status recovery", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
    state: "RUNNING",
  });
  const { messageId: toolId } = await apiClient.seedSessionMessage(sessionId, {
    type: "tool_call",
    authorType: "agent",
    content: "Compacting conversation",
    createdAt: "2026-09-30T00:00:00Z",
    metadata: {
      tool_call_id: "compaction-1",
      tool_name: "compact",
      tool_title: "Compact conversation",
      status: "running",
    },
  });
  await apiClient.seedSessionMessage(sessionId, {
    type: "status",
    content: "Still waiting on Compact conversation.",
    metadata: {
      action_visibility: "running",
      actions: [
        {
          type: "ws_request",
          label: "Cancel turn",
          test_id: "stall-cancel-turn-button",
          params: { method: "agent.cancel", payload: { session_id: sessionId } },
        },
      ],
    },
  });

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  const notice = session.activeChat().getByTestId("running-action-notice");
  await expect(notice).toHaveCount(1);
  await expect(notice).toContainText("Still waiting on Compact conversation.");
  await expect(session.agentStatus()).toBeVisible();
  await prCapture.screenshot("quiet-turn", {
    caption: "Synthetic running turn with a current compaction notice.",
  });

  await apiClient.updateSessionMessage(toolId, "Conversation compacted. Continuing work.");

  await expect(notice).toHaveCount(0);
  await expect(session.agentStatus()).toBeVisible();
  await assertNoDocumentHorizontalOverflow(testPage);
  await prCapture.screenshot("resumed-turn", {
    caption: "The same turn remains running after compaction activity clears the notice.",
  });

  await testPage.reload();
  await session.waitForLoad();
  await expect(session.agentStatus()).toBeVisible();
  await expect(notice).toHaveCount(0);
  await assertNoDocumentHorizontalOverflow(testPage);
});
