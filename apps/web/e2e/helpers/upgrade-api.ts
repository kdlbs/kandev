import { expect } from "@playwright/test";
import type { ApiClient } from "./api-client";

// v0.97.0 and current main share these production HTTP/WS request shapes.
// Capability errors stay fatal: no SQL seeding or current-backend fallback.
export async function waitForUpgradeTurn(
  api: ApiClient,
  taskId: string,
  sessionId: string,
  minimumResponses = 1,
) {
  await expect
    .poll(
      async () => {
        const messages = (await api.listSessionMessages(sessionId)).messages;
        return messages.filter((message) => message.content.includes("simple mock response"))
          .length;
      },
      { timeout: 90_000, message: `actual mock response for ${taskId}/${sessionId}` },
    )
    .toBeGreaterThanOrEqual(minimumResponses);
  await expect
    .poll(
      async () =>
        (await api.listTaskSessions(taskId)).sessions.find((s) => s.id === sessionId)?.state,
      { timeout: 90_000, message: `settled turn ${sessionId}` },
    )
    .toBe("WAITING_FOR_INPUT");
}
export async function sendUpgradeFollowup(api: ApiClient, taskId: string, sessionId: string) {
  const before = (await api.listSessionMessages(sessionId)).messages.filter((m) =>
    m.content.includes("simple mock response"),
  ).length;
  await api.addUserMessage(taskId, sessionId, "/e2e:simple-message");
  await waitForUpgradeTurn(api, taskId, sessionId, before + 1);
}
