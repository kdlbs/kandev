import { describe, expect, it } from "vitest";
import type { ApiClient } from "./api-client";
import {
  readSessionMessageIdsContaining,
  waitForNewSessionMessage,
} from "./session-resume-prompt-queue";

describe("resume provider acceptance witnesses", () => {
  it("does not count a user quoting the provider marker as provider output", async () => {
    const apiClient = {
      listSessionMessages: async () => ({
        messages: [
          { id: "user", author_type: "user", content: "started" },
          { id: "agent", author_type: "agent", content: "started" },
        ],
      }),
    } as unknown as ApiClient;
    expect(await readSessionMessageIdsContaining(apiClient, "session", "started")).toEqual(
      new Set(["agent"]),
    );
  });

  it("waits past a new user message until a new agent message arrives", async () => {
    const snapshots = [
      [{ id: "user", author_type: "user", content: "started" }],
      [{ id: "agent", author_type: "agent", content: "started" }],
    ];
    let reads = 0;
    const apiClient = {
      listSessionMessages: async () => ({
        messages: snapshots[Math.min(reads++, snapshots.length - 1)],
      }),
    } as unknown as ApiClient;
    await waitForNewSessionMessage(apiClient, "session", new Set(), "started", 2_000);
    expect(reads).toBe(2);
  });
});
