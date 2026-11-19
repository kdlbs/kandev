import { randomUUID } from "node:crypto";
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

// Following confirmed removal must not require a full document reload.
test("follows a stopped primary deleted from another client without reloading", async ({
  testPage,
  desktopTestPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.seedTask(seedData.workspaceId, "Remote primary removal", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const old = await apiClient.seedTaskSession(task.task_id, {
    sessionId: randomUUID(),
    agentProfileId: seedData.agentProfileId,
    state: "FAILED",
    completedAt: new Date().toISOString(),
    errorMessage: "Stored failure before removal",
  });
  const next = await apiClient.seedTaskSession(task.task_id, {
    sessionId: randomUUID(),
    agentProfileId: seedData.agentProfileId,
    state: "WAITING_FOR_INPUT",
  });
  await apiClient.seedSessionMessage(old.session_id, {
    type: "error",
    content: "Stored failure before removal",
  });
  await apiClient.setPrimarySession(old.session_id);
  const observer = new SessionPage(testPage);
  const deletingClient = new SessionPage(desktopTestPage);
  await testPage.goto(`/t/${task.task_id}`);
  await observer.waitForLoad();
  await expect(observer.activeChat()).toHaveAttribute("data-session-id", old.session_id);
  await desktopTestPage.goto(`/t/${task.task_id}`);
  await deletingClient.waitForLoad();
  await expect(deletingClient.activeChat()).toHaveAttribute("data-session-id", old.session_id);
  const before = (await apiClient.listTaskSessions(task.task_id)).sessions;
  expect(before.find((row) => row.is_primary)?.id).toBe(old.session_id);
  expect(before.find((row) => row.id === old.session_id)?.state).toBe("FAILED");
  const documentStart = await testPage.evaluate(() => performance.timeOrigin);

  await deletingClient.sessionTabBySessionId(old.session_id).click({ button: "right" });
  await deletingClient.contextMenuItem("Delete").click();
  const confirmation = desktopTestPage.getByTestId("session-delete-confirm-popover");
  await expect(confirmation).toBeVisible();
  await confirmation.getByTestId("session-delete-confirm").click();

  await expect
    .poll(async () => {
      const { sessions } = await apiClient.listTaskSessions(task.task_id);
      return sessions.map((row) => ({ id: row.id, primary: row.is_primary }));
    })
    .toEqual([{ id: next.session_id, primary: true }]);
  await expect(observer.activeChat()).toHaveAttribute("data-session-id", next.session_id, {
    timeout: 15_000,
  });
  await expect(observer.sessionTabBySessionId(old.session_id)).toHaveCount(0);
  expect(await testPage.evaluate(() => performance.timeOrigin)).toBe(documentStart);
});
