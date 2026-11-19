import { randomUUID } from "node:crypto";
import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { SessionPage } from "../pages/session-page";
import { watchWs } from "./causal-waits";

export async function expectLastStoppedSessionRemoval(
  observerPage: Page,
  deletingPage: Page,
  api: ApiClient,
  seed: SeedData,
): Promise<void> {
  const task = await api.seedTask(seed.workspaceId, "Remote last stopped session removal", {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
  });
  const old = await api.seedTaskSession(task.task_id, {
    sessionId: randomUUID(),
    agentProfileId: seed.agentProfileId,
    state: "FAILED",
    completedAt: new Date().toISOString(),
    errorMessage: "Last stored conversation",
  });
  await api.seedSessionMessage(old.session_id, {
    type: "error",
    content: "Last stored conversation",
  });
  await api.setPrimarySession(old.session_id);
  const observer = new SessionPage(observerPage);
  const deletingClient = new SessionPage(deletingPage);
  const watcher = watchWs(observerPage);
  await observerPage.goto(`/t/${task.task_id}`);
  await expect(observer.activeChat()).toHaveAttribute("data-session-id", old.session_id);
  await deletingPage.goto(`/t/${task.task_id}`);
  await expect(deletingClient.activeChat()).toHaveAttribute("data-session-id", old.session_id);
  const before = await api.listTaskSessions(task.task_id);
  expect(before.sessions.map((row) => ({ id: row.id, state: row.state }))).toEqual([
    { id: old.session_id, state: "FAILED" },
  ]);
  const documentStart = await observerPage.evaluate(() => performance.timeOrigin);
  const removed = watcher.waitForEvent("session.removed", {
    where: (payload) => payload.session_id === old.session_id,
  });
  await deletingClient.sessionTabBySessionId(old.session_id).click({ button: "right" });
  await deletingClient.contextMenuItem("Delete").click();
  const confirmation = deletingPage.getByTestId("session-delete-confirm-popover");
  await expect(confirmation).toBeVisible();
  await confirmation.getByTestId("session-delete-confirm").click();
  await removed;
  await expect.poll(async () => (await api.listTaskSessions(task.task_id)).sessions).toEqual([]);
  await expect(
    observerPage.locator(`[data-testid="session-chat"][data-session-id="${old.session_id}"]`),
  ).toHaveCount(0);
  await expect(observer.sessionTabBySessionId(old.session_id)).toHaveCount(0);
  await expect(observerPage.getByText("Last stored conversation", { exact: true })).toHaveCount(0);
  expect(await observerPage.evaluate(() => performance.timeOrigin)).toBe(documentStart);
}
