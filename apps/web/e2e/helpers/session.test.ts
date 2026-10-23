// @vitest-environment node
import { describe, expect, it, vi } from "vitest";
import type { Page } from "@playwright/test";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { seedIdleSession, waitForAgentMessage } from "./session";
import { waitForSessionSettledBaseline } from "../tests/chat/quick-chat-helpers";

vi.mock("../pages/session-page", () => ({
  SessionPage: class {
    async waitForLoad() {}
    async waitForChatIdle() {}
    async composerReady() {}
  },
}));

vi.mock("../fixtures/test-base", async () => ({ expect: (await import("vitest")).expect }));

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
      listSessionMessages: async () => ({
        messages: [{ author_type: "agent", content: "simple mock response" }],
      }),
      listTaskSessions: async () => ({
        sessions: [{ id: "session-1", state: "WAITING_FOR_INPUT" }],
      }),
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

  it("waits for the opening turn before accepting an idle Quick Chat baseline", async () => {
    const api = {
      listTaskSessions: vi.fn().mockResolvedValue({
        sessions: [{ id: "quick", state: "WAITING_FOR_INPUT" }],
      }),
      listSessionTurns: vi
        .fn()
        .mockResolvedValueOnce({ turns: [] })
        .mockResolvedValueOnce({ turns: [{ id: "opening", completed_at: null }] })
        .mockResolvedValue({ turns: [{ id: "opening", completed_at: "2026-10-15T00:00:00Z" }] }),
    };
    await waitForSessionSettledBaseline(api as unknown as ApiClient, "task", "quick");
    expect(api.listSessionTurns).toHaveBeenCalledTimes(3);
    expect(api.listSessionTurns).toHaveBeenLastCalledWith("quick");
  });

  it("retains the active-session fence when an earlier Quick Chat turn is complete", async () => {
    const api = {
      listTaskSessions: vi
        .fn()
        .mockResolvedValueOnce({ sessions: [{ id: "quick", state: "RUNNING" }] })
        .mockResolvedValue({ sessions: [{ id: "quick", state: "WAITING_FOR_INPUT" }] }),
      listSessionTurns: vi.fn().mockResolvedValue({
        turns: [{ id: "opening", completed_at: "2026-10-15T00:00:00Z" }],
      }),
    };
    await waitForSessionSettledBaseline(api as unknown as ApiClient, "task", "quick");
    expect(api.listTaskSessions).toHaveBeenCalledTimes(2);
    expect(api.listSessionTurns).toHaveBeenLastCalledWith("quick");
  });

  it("does not accept another session's settled state for the Quick Chat baseline", async () => {
    const api = {
      listTaskSessions: vi
        .fn()
        .mockResolvedValueOnce({ sessions: [{ id: "other", state: "WAITING_FOR_INPUT" }] })
        .mockResolvedValue({ sessions: [{ id: "quick", state: "WAITING_FOR_INPUT" }] }),
      listSessionTurns: vi.fn().mockResolvedValue({
        turns: [{ id: "opening", completed_at: "2026-10-15T00:00:00Z" }],
      }),
    };
    await waitForSessionSettledBaseline(api as unknown as ApiClient, "task", "quick");
    expect(api.listTaskSessions).toHaveBeenCalledTimes(2);
    expect(api.listSessionTurns).toHaveBeenLastCalledWith("quick");
  });
});
