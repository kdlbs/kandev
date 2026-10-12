import { describe, it, expect, vi, beforeEach } from "vitest";

const { fetchJson } = vi.hoisted(() => ({ fetchJson: vi.fn() }));
vi.mock("../client", () => ({ fetchJson }));

const requestMock = vi.fn();
let wsStatus = "connected";
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({
    getStatus: () => wsStatus,
    request: requestMock,
  }),
}));

const {
  listBackgroundWorkloads,
  getBackgroundWorkload,
  executeBackgroundAction,
  getBackgroundWorkloadUsage,
} = await import("./background-work-api");

describe("background-work-api", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    wsStatus = "connected";
    requestMock.mockResolvedValue({ success: true, work_id: "work-1", action: "stop" });
    fetchJson.mockResolvedValue({ workloads: [] });
  });

  it("lists background workloads for a session", async () => {
    await listBackgroundWorkloads("session-1");
    expect(fetchJson).toHaveBeenCalledWith(
      "/api/v1/task-sessions/session-1/background-work",
      undefined,
    );
  });

  it("gets single background workload", async () => {
    await getBackgroundWorkload("session-1", "work-1");
    expect(fetchJson).toHaveBeenCalledWith(
      "/api/v1/task-sessions/session-1/background-work/work-1",
      undefined,
    );
  });

  it("executes action via WebSocket when connected", async () => {
    const res = await executeBackgroundAction("session-1", {
      work_id: "work-1",
      action: "stop",
      operation_id: "op-1",
    });

    expect(requestMock).toHaveBeenCalledWith(
      "session.background_work.action",
      {
        session_id: "session-1",
        work_id: "work-1",
        action: "stop",
        operation_id: "op-1",
      },
      10000,
    );
    expect(res).toEqual({ success: true, work_id: "work-1", action: "stop" });
  });

  it("reports uncertain error and does not fall back to HTTP when WebSocket request fails", async () => {
    requestMock.mockRejectedValueOnce(new Error("socket timeout"));

    const res = await executeBackgroundAction("session-1", {
      work_id: "work-1",
      action: "stop",
      operation_id: "op-1",
    });

    expect(requestMock).toHaveBeenCalled();
    expect(fetchJson).not.toHaveBeenCalled();
    expect(res.success).toBe(false);
    expect(res.uncertain).toBe(true);
    expect(res.error).toContain("socket timeout");
  });

  it("uses HTTP POST when WebSocket is disconnected", async () => {
    wsStatus = "disconnected";
    fetchJson.mockResolvedValueOnce({ success: true, work_id: "work-1", action: "stop" });

    const res = await executeBackgroundAction("session-1", {
      work_id: "work-1",
      action: "stop",
      operation_id: "op-1",
    });

    expect(requestMock).not.toHaveBeenCalled();
    expect(fetchJson).toHaveBeenCalledWith(
      "/api/v1/task-sessions/session-1/background-work/work-1/action",
      expect.objectContaining({
        init: expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ work_id: "work-1", action: "stop", operation_id: "op-1" }),
        }),
      }),
    );
    expect(res).toEqual({ success: true, work_id: "work-1", action: "stop" });
  });

  it("fetches background workload usage", async () => {
    fetchJson.mockResolvedValueOnce({
      work_id: "work-1",
      tokens_in: 100,
      tokens_out: 200,
      tokens_total: 300,
      cost_subcents: 15,
      provenance: "reported",
    });

    const res = await getBackgroundWorkloadUsage("session-1", "work-1");
    expect(fetchJson).toHaveBeenCalledWith(
      "/api/v1/task-sessions/session-1/background-work/work-1/usage",
      undefined,
    );
    expect(res.tokens_total).toBe(300);
  });
});
