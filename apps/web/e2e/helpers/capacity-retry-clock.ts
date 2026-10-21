import type { BackendContext } from "../fixtures/backend";
import fs from "node:fs";
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
) {
  for (let attempt = 1; attempt <= count; attempt++) {
    const notice = await pollUntil(
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
    ).catch(async (cause) => {
      console.error(
        fs
          .readFileSync(backend.logPath, "utf8")
          .split("\n")
          .filter((line) =>
            /capacity continuation retry acceptance|refusing automatic transient retry/.test(line),
          )
          .slice(-10)
          .join("\n"),
      );
      const { messages } = await apiClient.listSessionMessages(sessionId);
      const recovery = messages
        .filter((message) => message.metadata?.retrying || message.metadata?.recovery_disposition)
        .map((message) => message.metadata);
      throw new Error(`Capacity retry ${attempt} missing: ${JSON.stringify(recovery)}`, { cause });
    });
    await advanceCapacityRetries(backend, Number(notice!.metadata!.retry_in_seconds));
  }
}
