import { test, expect, type Page } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { dwell } from "../../helpers/causal-waits";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

type E2ESessionStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => {
      messages: { bySession: Record<string, unknown[] | undefined> };
      taskSessions: { items: Record<string, unknown> };
    };
  };
};

async function cachedSessionState(
  page: Page,
  sessionId: string,
): Promise<{ messages: number | null; session: boolean }> {
  return page.evaluate((sid) => {
    const store = (window as E2ESessionStoreWindow).__KANDEV_E2E_STORE__;
    if (!store) throw new Error("E2E store bridge is unavailable");
    const state = store.getState();
    return {
      messages: state.messages.bySession[sid]?.length ?? null,
      session: sid in state.taskSessions.items,
    };
  }, sessionId);
}

test.describe("deleted task client state", () => {
  // @covers AC-UI-DELETED-TASK-CLIENT-STATE-001.4
  test("stops Git read recovery after the task is deleted", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Deleted Git reads",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await expect
      .poll(async () => (await apiClient.listTaskSessions(task.id)).sessions.length)
      .toBeGreaterThan(0);
    const sessionId = (await apiClient.listTaskSessions(task.id)).sessions[0].id;
    let gitReads = 0;
    let deleted = false;
    await testPage.routeWebSocket(/\/ws$/, (socket) => {
      const server = socket.connectToServer();
      socket.onMessage((message) => {
        if (typeof message !== "string") return server.send(message);
        for (const part of message.split("\n").filter(Boolean)) {
          const frame = JSON.parse(part) as {
            id?: string;
            type?: string;
            action?: string;
            payload?: { session_id?: string };
          };
          if (
            frame.type === "request" &&
            frame.payload?.session_id === sessionId &&
            (frame.action === "session.git.commits" || frame.action === "session.cumulative_diff")
          ) {
            gitReads++;
            socket.send(
              JSON.stringify({
                ...frame,
                type: "error",
                payload: { code: "UNAVAILABLE", message: "temporary" },
              }),
            );
          } else server.send(part);
        }
      });
      server.onMessage((message) => {
        if (typeof message === "string") {
          for (const part of message.split("\n").filter(Boolean)) {
            const frame = JSON.parse(part) as { action?: string; payload?: { task_id?: string } };
            if (frame.action === "task.deleted" && frame.payload?.task_id === task.id)
              deleted = true;
          }
        }
        socket.send(message);
      });
    });
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.clickTab("Changes");
    await expect.poll(() => gitReads, { timeout: 15_000 }).toBeGreaterThanOrEqual(2);
    const initialReads = gitReads;
    await expect.poll(() => gitReads, { timeout: 15_000 }).toBeGreaterThan(initialReads);
    await expect
      .poll(async () => (await cachedSessionState(testPage, sessionId)).session)
      .toBe(true);
    await apiClient.deleteTask(task.id);
    await expect.poll(() => deleted).toBe(true);
    await expect
      .poll(async () => (await cachedSessionState(testPage, sessionId)).session)
      .toBe(false);
    const readsAfterDeletion = gitReads;
    await dwell(testPage, 6500, "negative-assertion", "deleted session Git reads must not retry");
    expect(gitReads).toBe(readsAfterDeletion);
  });

  // @covers AC-UI-DELETED-TASK-CLIENT-STATE-001.1
  // @covers AC-UI-DELETED-TASK-CLIENT-STATE-001.2
  test("drops the deleted task's transcript and keeps other tasks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const createTask = (title: string) =>
      apiClient.createTaskWithAgent(seedData.workspaceId, title, seedData.agentProfileId, {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      });
    const kept = await createTask("Kept task");
    const deleted = await createTask("Deleted task");

    const settledSessionId = async (taskId: string) => {
      await expect
        .poll(
          async () =>
            DONE_STATES.includes((await apiClient.listTaskSessions(taskId)).sessions[0]?.state),
          { timeout: 30_000, message: `session for ${taskId} did not settle` },
        )
        .toBe(true);
      return (await apiClient.listTaskSessions(taskId)).sessions[0].id;
    };
    const keptSessionId = await settledSessionId(kept.id);
    const deletedSessionId = await settledSessionId(deleted.id);

    const session = new SessionPage(testPage);
    const waitForCachedMessages = (sessionId: string) =>
      expect
        .poll(async () => (await cachedSessionState(testPage, sessionId)).messages ?? 0)
        .toBeGreaterThan(0);

    await testPage.goto(`/t/${deleted.id}`);
    await session.waitForLoad();
    await waitForCachedMessages(deletedSessionId);

    await session.sidebarTaskItem("Kept task").click();
    await expect(session.activeSidebarTaskItem("Kept task")).toBeVisible();
    await waitForCachedMessages(keptSessionId);
    expect((await cachedSessionState(testPage, deletedSessionId)).messages).toBeGreaterThan(0);

    await apiClient.deleteTask(deleted.id);

    await expect
      .poll(() => cachedSessionState(testPage, deletedSessionId), {
        message: "deleted task's session state stayed in the store",
      })
      .toEqual({ messages: null, session: false });
    const keptState = await cachedSessionState(testPage, keptSessionId);
    expect(keptState.session).toBe(true);
    expect(keptState.messages).toBeGreaterThan(0);
  });
});
