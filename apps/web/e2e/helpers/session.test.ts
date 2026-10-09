// @vitest-environment node
import { describe, expect, it, vi } from "vitest";
import type { Page } from "@playwright/test";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { seedIdleSession, waitForAgentMessage } from "./session";

vi.mock("../pages/session-page", () => ({
  SessionPage: class {
    async waitForLoad() {}
    async waitForChatIdle() {}
    async composerReady() {}
  },
}));

describe("session causal helpers", () => {
  it("does not seed an idle session until the initial turn is durably complete", async () => {
    type Turns = Awaited<ReturnType<ApiClient["listSessionTurns"]>>;
    let completeInitialTurn!: (value: Turns) => void;
    const initialTurn = new Promise<Turns>((resolve) => {
      completeInitialTurn = resolve;
    });
    const listSessionTurns = vi
      .fn(() => initialTurn)
      .mockResolvedValueOnce({ turns: [] })
      .mockResolvedValueOnce({ turns: [{ id: "initial-turn" }] });
    const apiClient = {
      createTaskWithAgent: async () => ({ id: "task-1", session_id: "session-1" }),
      listSessionTurns,
    } as unknown as ApiClient;
    const page = { goto: async () => undefined } as unknown as Page;
    const seedData = {} as SeedData;
    let ready = false;
    const seeded = seedIdleSession(page, apiClient, seedData, "retry fixture").then(() => {
      ready = true;
    });

    try {
      await vi.waitFor(() => expect(listSessionTurns).toHaveBeenCalledTimes(3), { timeout: 5_000 });
      expect(listSessionTurns).toHaveBeenCalledWith("session-1");
      expect(ready).toBe(false);
    } finally {
      completeInitialTurn({
        turns: [{ id: "initial-turn", completed_at: "2026-10-09T00:00:00Z" }],
      });
      await seeded;
    }
    expect(ready).toBe(true);
  }, 10_000);

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
