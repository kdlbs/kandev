import { describe, expect, it, vi } from "vitest";
import type { ApiClient } from "./api-client";
import { waitForAgentMessage } from "./session";
import { waitForSessionSettledBaseline } from "../tests/chat/quick-chat-helpers";

vi.mock("../fixtures/test-base", async () => ({ expect: (await import("vitest")).expect }));

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

  it("waits for the opening turn before accepting an idle Quick Chat baseline", async () => {
    const startupTurn = {
      id: "startup",
      completed_at: "2026-10-15T00:00:00Z",
      metadata: { lifecycle_only: true },
    };
    const api = {
      listTaskSessions: vi.fn().mockResolvedValue({
        sessions: [{ id: "quick", state: "WAITING_FOR_INPUT" }],
      }),
      listSessionTurns: vi
        .fn()
        .mockResolvedValueOnce({ turns: [] })
        .mockResolvedValueOnce({ turns: [startupTurn] })
        .mockResolvedValueOnce({ turns: [startupTurn, { id: "opening", completed_at: null }] })
        .mockResolvedValue({
          turns: [startupTurn, { id: "opening", completed_at: "2026-10-15T00:00:01Z" }],
        }),
    };
    await waitForSessionSettledBaseline(api as unknown as ApiClient, "task", "quick");
    expect(api.listSessionTurns).toHaveBeenCalledTimes(4);
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
