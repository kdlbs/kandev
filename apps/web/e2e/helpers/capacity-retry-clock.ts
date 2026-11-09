import type { BackendContext } from "../fixtures/backend";
import type { ApiClient } from "./api-client";
import { pollUntil } from "./poll-until";

export async function freezeCapacityRetries(backend: BackendContext) {
  const response = await fetch(`${backend.baseUrl}/api/v1/e2e/capacity-retry-clock/freeze`, {
    method: "POST",
  });
  if (!response.ok) throw new Error(`Cannot freeze retry clock: ${response.status}`);
}

export async function advanceCapacityRetries(backend: BackendContext, seconds: number) {
  const response = await fetch(`${backend.baseUrl}/api/v1/e2e/capacity-retry-clock/advance`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ seconds }),
  });
  if (!response.ok) throw new Error(`Cannot advance retry clock: ${response.status}`);
}

export async function driveCapacityRetries(
  backend: BackendContext,
  apiClient: ApiClient,
  sessionId: string,
  count: number,
  waitForLastRetry?: () => Promise<unknown>,
) {
  for (let attempt = 1; attempt <= count; attempt++) {
    const notice = await waitForCapacityRetry(apiClient, sessionId, attempt);
    const waiting = attempt === count ? waitForLastRetry?.() : undefined;
    await Promise.all([
      waiting,
      advanceCapacityRetries(backend, Number(notice!.metadata!.retry_in_seconds)),
    ]);
  }
}

export async function waitForCapacityRetry(
  apiClient: ApiClient,
  sessionId: string,
  attempt: number,
) {
  return pollUntil(
    async () => {
      const { messages } = await apiClient.listSessionMessages(sessionId);
      return messages.find(
        (message) =>
          message.metadata?.retrying === true &&
          message.metadata.recovery_phase === "waiting" &&
          message.metadata.attempt === attempt,
      );
    },
    (value) => value !== undefined,
    30_000,
    `waiting for capacity retry ${attempt}`,
  );
}
