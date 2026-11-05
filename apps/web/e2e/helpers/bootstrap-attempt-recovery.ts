import { expect, test } from "../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";

export function bootstrapAttemptRecoveryScenario() {
  test("keeps failed bootstrap work on its session and sends the displayed recovery stamp", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Bootstrap recovery ownership", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      agent_profile_id: seedData.agentProfileId,
    });
    const stamp = "bootstrap-attempt-fixture";
    const occurredAt = new Date().toISOString();
    const causes = [{ operation: "start", code: "authentication_required", prompt_not_sent: true }];
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
      errorMessage: "The agent could not start.",
      metadata: {
        last_agent_error: {
          message: "The agent could not start.",
          stamp,
          occurred_at: occurredAt,
          scope: "session",
          phase: "bootstrap",
          code: "generic_launch_failure",
          execution_id: "failed-execution",
          attempt_id: "failed-attempt",
          causes,
        },
      },
    });
    await apiClient.seedSessionMessage(sessionId, {
      type: "status",
      content: "The agent could not start.",
      createdAt: occurredAt,
      metadata: {
        recovery_actions: true,
        scope: "session",
        phase: "bootstrap",
        error_stamp: stamp,
        failure_kind: "generic_launch_failure",
        causes,
        actions: [
          {
            type: "ws_request",
            label: "Resume session",
            params: {
              method: "session.recover",
              payload: { task_id: task.id, session_id: sessionId, action: "resume" },
            },
          },
        ],
      },
    });
    const identity = await apiClient.getQueueSessionIdentity(task.id, sessionId);
    await apiClient.queueMessage(identity, "Keep this prompt on the failed session");
    const recoveryRequests: Array<{ stamp?: string; sessionId: string }> = [];
    testPage.on("websocket", (socket) => {
      socket.on("framesent", ({ payload }) => {
        if (typeof payload !== "string") return;
        for (const line of payload.split("\n").filter(Boolean)) {
          const frame = JSON.parse(line);
          if (
            frame.type === "request" &&
            frame.action === "session.recover" &&
            frame.payload?.session_id === sessionId
          ) {
            recoveryRequests.push({
              stamp: frame.payload.error_stamp,
              sessionId: frame.payload.session_id,
            });
          }
        }
      });
    });
    await testPage.goto(`/t/${task.id}`);
    const card = testPage.getByTestId("session-recovery-card");
    await expect(card).toBeVisible();
    const resume = card.getByTestId("recovery-resume-button");
    await expect(resume).toBeEnabled();
    if (testInfo.project.name === "mobile-chrome") {
      expect((await resume.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    }
    await resume.click();
    await expect.poll(() => recoveryRequests).toEqual([{ stamp, sessionId }]);
    await expect(card.getByTestId("session-recovery-error")).toBeVisible();
    await expect(resume).toBeEnabled();
    await expect(card).toBeVisible();
    const queue = await apiClient.getQueueStatus(identity);
    expect(queue.entries.map((entry) => entry.content)).toEqual([
      "Keep this prompt on the failed session",
    ]);
    await testPage.getByTestId("queue-chip").click();
    await expect(testPage.getByTestId("queued-ghost-list")).toContainText(
      "Keep this prompt on the failed session",
    );
    await assertNoDocumentHorizontalOverflow(testPage, "bootstrap recovery");
    await testPage.reload();
    await expect(card).toBeVisible();
    const { sessions } = await apiClient.listTaskSessions(task.id);
    expect(sessions.map((session) => session.id)).toEqual([sessionId]);
    expect((await apiClient.getQueueStatus(identity)).count).toBe(1);
  });
}
