import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { injectLatency } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";

export async function expectQueueBusyRetry(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  mobile: boolean,
  captureBusy?: () => Promise<void>,
): Promise<void> {
  let refused = false;
  await page.routeWebSocket(/\/ws$/, (socket) => {
    const server = socket.connectToServer();
    socket.onMessage(async (message) => {
      if (typeof message === "string") {
        const frame = JSON.parse(message) as { type?: string; id?: string; action?: string };
        if (!refused && frame.type === "request" && frame.action === "message.queue.cancel") {
          refused = true;
          await injectLatency(
            6_000,
            "queue busy response arrives after the default transport timeout",
          );
          socket.send(
            JSON.stringify({
              type: "error",
              id: frame.id,
              action: frame.action,
              payload: {
                code: "UNAVAILABLE",
                message: "Session queue is busy; retry the operation",
              },
            }),
          );
          return;
        }
      }
      server.send(message);
    });
    server.onMessage((message) => socket.send(message));
  });
  const task = await apiClient.createTask(seedData.workspaceId, "Queue busy retry", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    agent_profile_id: seedData.agentProfileId,
    repository_ids: [seedData.repositoryId],
  });
  const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
    state: "IDLE",
    agentProfileId: seedData.agentProfileId,
  });
  const identity = await apiClient.getQueueSessionIdentity(task.id, sessionId);
  await apiClient.setQueueAutoRun(identity, false);
  for (let index = 0; index < 10; index++) {
    await apiClient.queueMessage(identity, `Preserved queue entry ${index + 1}`);
  }
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  const chat = session.activeChat();
  await chat.getByTestId("queue-chip").click();
  const clear = chat.getByTestId("queue-clear-all");
  await expect(clear).toBeEnabled();
  if (mobile) await clear.tap();
  else await clear.click();
  await expect(
    page.getByText("The message queue is temporarily unavailable. Try again.", { exact: true }),
  ).toBeVisible({ timeout: 15_000 });
  expect((await apiClient.getQueueStatus(identity)).count).toBe(10);
  // Clear all closes the panel optimistically; refusal restores its queue chip.
  const restoredChip = chat.getByTestId("queue-chip");
  await expect(restoredChip).toBeVisible();
  if (mobile) await restoredChip.tap();
  else await restoredChip.click();
  await expect(chat.getByTestId("queue-entry")).toHaveCount(10);
  await captureBusy?.();
  await expect(clear).toBeEnabled();
  if (mobile) await clear.tap();
  else await clear.click();
  await expect.poll(async () => (await apiClient.getQueueStatus(identity)).count).toBe(0);
  await expect(chat.getByTestId("queue-entry")).toHaveCount(0);
}
