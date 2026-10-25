import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const FIRST_SNAPSHOT = "# Batch ES reads\n\n1. Read the first index";
const LATEST_SNAPSHOT =
  "# Batch ES reads\n\n1. Read the first index\n2. Read the second index\n3. Merge results";

test.describe("agent plan coalescing", () => {
  // The focused unit RED proves the projection defect; this browser case proves
  // persisted replay through the shared desktop/phone transcript path.
  // @covers AC-AGENTS-AGENT-PLAN-STREAM-COALESCING-001.3
  test("shows one latest card for persisted cumulative plan snapshots", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Agent plan coalescing", {
      description: "Persisted cumulative agent plan snapshots",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
      state: "IDLE",
      agentProfileId: seedData.agentProfileId,
    });
    await apiClient.seedSessionMessage(sessionId, {
      type: "agent_plan",
      content: FIRST_SNAPSHOT,
    });
    await apiClient.seedSessionMessage(sessionId, {
      type: "agent_plan",
      content: LATEST_SNAPSHOT,
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const chat = session.activeChat();
    await expect(chat.getByText("Batch ES reads", { exact: true })).toHaveCount(1);
    await expect(chat.getByText("Merge results", { exact: true })).toBeVisible();

    await testPage.reload();
    await session.waitForLoad();
    await expect(session.activeChat().getByText("Batch ES reads", { exact: true })).toHaveCount(1);
    await expect(session.activeChat().getByText("Merge results", { exact: true })).toBeVisible();

    await prCapture.screenshot("desktop-agent-plan-coalescing", {
      caption: "Desktop transcript shows one latest agent plan card after replay.",
    });
    if (prCapture.capturing) {
      await testPage.setViewportSize({ width: 393, height: 851 });
      await expect(session.activeChat().getByText("Batch ES reads", { exact: true })).toHaveCount(
        1,
      );
      await prCapture.screenshot("phone-agent-plan-coalescing", {
        caption: "Phone transcript uses the same single-card agent plan projection.",
      });
    }
  });
});
