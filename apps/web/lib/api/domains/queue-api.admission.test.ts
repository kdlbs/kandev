import { beforeEach, describe, expect, it, vi } from "vitest";
import { WebSocketRequestError } from "@/lib/ws/request-error";

const getWebSocketClientMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: getWebSocketClientMock,
}));

import { QueueAdmissionError, QueueFullError, queueMessage } from "./queue-api";

const baseParams = {
  session_id: "session-1",
  session_incarnation_id: "incarnation-1",
  task_id: "task-1",
  content: "queued prompt",
  client_queue_id: "client-queue-1",
};
const QUEUE_ADD_ACTION = "message.queue.add";
const QUEUE_GET_ACTION = "message.queue.get";
const MESSAGE_LIST_ACTION = "message.list";
const ATTACHMENT_ID = "file-1";

beforeEach(() => {
  getWebSocketClientMock.mockReset();
});

describe("identified queue admission", () => {
  it("forwards an ordinary admission with the ten second budget", async () => {
    const request = vi.fn().mockResolvedValue({ id: baseParams.client_queue_id });
    getWebSocketClientMock.mockReturnValue({ request });

    await queueMessage(baseParams);

    expect(request).toHaveBeenCalledWith(QUEUE_ADD_ACTION, baseParams, 10000);
  });

  it("uses the thirty second budget for attachment admission", async () => {
    const request = vi.fn().mockResolvedValue({ id: baseParams.client_queue_id });
    getWebSocketClientMock.mockReturnValue({ request });
    const params = {
      ...baseParams,
      attachments: [{ type: "resource", mime_type: "text/plain", attachment_id: ATTACHMENT_ID }],
    };

    await queueMessage(params);

    expect(request).toHaveBeenCalledWith(QUEUE_ADD_ACTION, params, 30000);
  });

  it("retries an uncertain ordinary admission once with the same budget and payload", async () => {
    let adds = 0;
    const request = vi.fn(async (action: string, ..._args: unknown[]) => {
      if (action === QUEUE_ADD_ACTION) {
        adds++;
        if (adds === 1) throw new Error("WebSocket request timed out");
        return { id: baseParams.client_queue_id };
      }
      if (action === QUEUE_GET_ACTION) return { entries: [], count: 0, max: 10 };
      if (action === MESSAGE_LIST_ACTION) return { messages: [], has_more: false };
      return undefined;
    });
    getWebSocketClientMock.mockReturnValue({
      request,
      getStatus: () => "connected",
    });

    await expect(queueMessage(baseParams)).resolves.toEqual({ id: baseParams.client_queue_id });

    const admissions = request.mock.calls.filter(([action]) => action === QUEUE_ADD_ACTION);
    expect(admissions).toEqual([
      [QUEUE_ADD_ACTION, baseParams, 10000],
      [QUEUE_ADD_ACTION, baseParams, 10000],
    ]);
  });

  it("uses the attachment budget for an uncertain retry", async () => {
    const params = {
      ...baseParams,
      attachments: [{ type: "resource", mime_type: "text/plain", attachment_id: ATTACHMENT_ID }],
    };
    let adds = 0;
    const request = vi.fn(async (action: string, ..._args: unknown[]) => {
      if (action === QUEUE_ADD_ACTION) {
        adds++;
        if (adds === 1) throw new Error("WebSocket request timed out");
        return { id: baseParams.client_queue_id };
      }
      if (action === QUEUE_GET_ACTION) return { entries: [], count: 0, max: 10 };
      if (action === MESSAGE_LIST_ACTION) return { messages: [], has_more: false };
      return undefined;
    });
    getWebSocketClientMock.mockReturnValue({
      request,
      getStatus: () => "connected",
    });

    await expect(queueMessage(params)).resolves.toEqual({ id: baseParams.client_queue_id });

    const admissions = request.mock.calls.filter(([action]) => action === QUEUE_ADD_ACTION);
    expect(admissions[0]?.[2]).toBe(30000);
    expect(admissions[1]?.[2]).toBe(30000);
  });

  it("does not scan older transcript pages for an ordinary admission", async () => {
    let adds = 0;
    const request = vi.fn(async (action: string, ..._args: unknown[]) => {
      if (action === QUEUE_ADD_ACTION) {
        adds++;
        if (adds === 1) throw new Error("WebSocket request timed out");
        return { id: baseParams.client_queue_id };
      }
      if (action === QUEUE_GET_ACTION) return { entries: [], count: 0, max: 10 };
      if (action === MESSAGE_LIST_ACTION) {
        return { messages: [], has_more: true, cursor: "older-page" };
      }
      return undefined;
    });
    getWebSocketClientMock.mockReturnValue({ request, getStatus: () => "connected" });

    await queueMessage(baseParams);

    expect(request).not.toHaveBeenCalledWith(
      MESSAGE_LIST_ACTION,
      expect.objectContaining({ before: "older-page" }),
      expect.anything(),
    );
  });
});

describe("identified queue admission errors", () => {
  it("maps deterministic queue rejection codes to typed errors", async () => {
    const cases = [
      ["queue_full", QueueFullError],
      ["queue_id_conflict", QueueAdmissionError],
      ["queue_session_unavailable", QueueAdmissionError],
      ["queue_admission_unavailable", QueueAdmissionError],
      ["VALIDATION_ERROR", QueueAdmissionError],
    ] as const;
    for (const [code, errorType] of cases) {
      const request = vi.fn().mockRejectedValue(new WebSocketRequestError("rejected", code));
      getWebSocketClientMock.mockReturnValue({ request });

      await expect(queueMessage(baseParams)).rejects.toBeInstanceOf(errorType);
    }
  });

  it("does not retry an explicit rejection", async () => {
    const request = vi
      .fn()
      .mockRejectedValue(new WebSocketRequestError("invalid queue request", "VALIDATION_ERROR"));
    getWebSocketClientMock.mockReturnValue({ request });

    await expect(queueMessage(baseParams)).rejects.toMatchObject({
      name: "QueueAdmissionError",
      code: "validation",
    });
    expect(request).toHaveBeenCalledTimes(1);
  });
});
