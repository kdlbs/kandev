import { test, expect, type Page } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

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
    for (const [taskId, sessionId] of [
      [kept.id, keptSessionId],
      [deleted.id, deletedSessionId],
    ] as const) {
      await testPage.goto(`/t/${taskId}`);
      await session.waitForLoad();
      await expect
        .poll(async () => (await cachedSessionState(testPage, sessionId)).messages ?? 0)
        .toBeGreaterThan(0);
    }

    await testPage.goto(`/t/${kept.id}`);
    await session.waitForLoad();
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
