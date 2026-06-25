import { test, expect } from "../../fixtures/test-base";
import { openTaskSession } from "../../helpers/session";
import { computeWsDrops, formatDroppedEvents, readWsAccount } from "../../helpers/ws-account";

test.describe("WS event accounting", () => {
  test("installs the browser hook and reports no backend/frontend drops", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "WS accounting smoke",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) {
      throw new Error("createTaskWithAgent did not return a session_id");
    }

    const session = await openTaskSession(testPage, task.id);
    await session.waitForChatIdle({ timeout: 30_000 });
    await expect(session.activeChat()).toBeVisible();

    await expect
      .poll(
        async () => {
          const snapshot = await readWsAccount(testPage);
          const sessionSnapshot = snapshot?.bySession[task.session_id ?? ""];

          if (!snapshot?.connectionId) return "missing-connection";
          if (snapshot.processedSeqs.length === 0) return "missing-connection-events";
          if (!sessionSnapshot || sessionSnapshot.processedSeqs.length === 0) {
            return "missing-session-events";
          }
          if (snapshot.gaps.length > 0) return `connection-gaps:${snapshot.gaps.join(",")}`;
          if (sessionSnapshot.gaps.length > 0) {
            return `session-gaps:${sessionSnapshot.gaps.join(",")}`;
          }
          return "ready";
        },
        {
          message: "expected stamped WS frames to be recorded by the browser hook",
          timeout: 15_000,
        },
      )
      .toBe("ready");

    const snapshot = await readWsAccount(testPage);
    expect(snapshot?.connectionId).toBeTruthy();

    const connectionSent = await apiClient.getWsSent(
      snapshot!.connectionId!,
      snapshot!.minSeq === null ? undefined : Math.max(0, snapshot!.minSeq - 1),
    );
    expect(connectionSent.events.length).toBeGreaterThan(0);

    const sessionSent = await apiClient.getWsSent(
      snapshot!.connectionId!,
      undefined,
      task.session_id,
    );
    expect(sessionSent.events.length).toBeGreaterThan(0);

    const drops = await computeWsDrops(testPage, apiClient, { strict: true });
    expect(drops, formatDroppedEvents(drops)).toHaveLength(0);
  });
});
