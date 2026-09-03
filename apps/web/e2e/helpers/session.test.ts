import { describe, expect, it } from "vitest";
import type { ApiClient } from "./api-client";
import { waitForAgentMessage } from "./session";

describe("session causal helpers", () => {
  it("waits for the expected agent message rather than any earlier transcript activity", async () => {
    const snapshots = [
      [],
      [{ author_type: "user", content: "started" }],
      [{ author_type: "agent", content: "started" }],
    ];
    let reads = 0;
    const apiClient = {
      listSessionMessages: async () => ({
        messages: snapshots[Math.min(reads++, snapshots.length - 1)]!,
      }),
    } as unknown as ApiClient;

    await waitForAgentMessage(apiClient, "session-1", "started", 2_000);

    expect(reads).toBe(3);
  });
});
