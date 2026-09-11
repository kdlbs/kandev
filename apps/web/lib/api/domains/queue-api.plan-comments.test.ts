/* eslint-disable sonarjs/no-duplicate-string -- Wire actions and fixture IDs are intentionally repeated for readability. */
import { beforeEach, describe, expect, it, vi } from "vitest";

const getWebSocketClientMock = vi.hoisted(() => vi.fn());
const QUEUE_ADD_ACTION = "message.queue.add";
const PRIMARY_SESSION_ID = "session-primary";
const CLIENT_QUEUE_ID = "client-queue-1";
const INCARNATION_ID = "incarnation-1";

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: getWebSocketClientMock,
}));

import { queueMessage } from "./queue-api";

beforeEach(() => {
  getWebSocketClientMock.mockReset();
});

// eslint-disable-next-line max-lines-per-function -- Admission retries share one wire-contract fixture.
describe("task plan queue admission", () => {
  it("forwards idempotent task plan comment admission fields", async () => {
    const request = vi.fn().mockResolvedValue({ id: "q-1" });
    getWebSocketClientMock.mockReturnValue({ request });

    await queueMessage({
      session_id: PRIMARY_SESSION_ID,
      session_incarnation_id: INCARNATION_ID,
      task_id: "task-1",
      client_queue_id: CLIENT_QUEUE_ID,
      content: "",
      plan_mode: true,
      plan_comment_refs: [{ id: "comment-1", version: 2 }],
      require_primary_session: true,
    });

    expect(request).toHaveBeenCalledWith(QUEUE_ADD_ACTION, {
      session_id: PRIMARY_SESSION_ID,
      session_incarnation_id: INCARNATION_ID,
      task_id: "task-1",
      client_queue_id: CLIENT_QUEUE_ID,
      content: "",
      plan_mode: true,
      plan_comment_refs: [{ id: "comment-1", version: 2 }],
      require_primary_session: true,
    });
  });

  it("reconciles a timed-out comment queue admission from queue state", async () => {
    const queued = {
      id: CLIENT_QUEUE_ID,
      session_id: PRIMARY_SESSION_ID,
      task_id: "task-1",
      content: "resolved",
      plan_mode: true,
      queued_at: "2026-09-02T00:00:00Z",
    };
    const request = vi.fn(async (action: string) => {
      if (action === QUEUE_ADD_ACTION) throw new Error("WebSocket request timed out");
      if (action === "message.queue.get") {
        return { entries: [queued], count: 1, max: 10, merge_enabled: true };
      }
      return undefined;
    });
    getWebSocketClientMock.mockReturnValue({ request });

    await expect(
      queueMessage({
        session_id: PRIMARY_SESSION_ID,
        session_incarnation_id: INCARNATION_ID,
        task_id: "task-1",
        client_queue_id: CLIENT_QUEUE_ID,
        content: "",
        plan_comment_refs: [{ id: "comment-1", version: 2 }],
      }),
    ).resolves.toEqual(queued);
    expect(request).toHaveBeenCalledWith("message.queue.get", {
      task_id: "task-1",
      session_id: PRIMARY_SESSION_ID,
      session_incarnation_id: INCARNATION_ID,
    });
  });

  it("reconciles an already-drained admission from transcript metadata", async () => {
    const request = vi.fn(async (action: string) => {
      if (action === QUEUE_ADD_ACTION) throw new Error("WebSocket request timed out");
      if (action === "message.queue.get") {
        return { entries: [], count: 0, max: 10, merge_enabled: true };
      }
      if (action === "message.list") {
        return {
          messages: [
            {
              id: "message-1",
              session_id: PRIMARY_SESSION_ID,
              task_id: "task-1",
              content: "resolved",
              metadata: { client_queue_id: CLIENT_QUEUE_ID },
              created_at: "2026-09-02T00:00:00Z",
            },
          ],
        };
      }
      return undefined;
    });
    getWebSocketClientMock.mockReturnValue({ request });

    await expect(
      queueMessage({
        session_id: PRIMARY_SESSION_ID,
        session_incarnation_id: INCARNATION_ID,
        task_id: "task-1",
        client_queue_id: CLIENT_QUEUE_ID,
        content: "",
        plan_mode: true,
        plan_comment_refs: [{ id: "comment-1", version: 2 }],
      }),
    ).resolves.toMatchObject({ id: CLIENT_QUEUE_ID, content: "resolved" });
  });

  it("reconciles an admission folded into a Send Now transcript", async () => {
    const request = vi.fn(async (action: string) => {
      if (action === QUEUE_ADD_ACTION) throw new Error("WebSocket request timed out");
      if (action === "message.queue.get") {
        return { entries: [], count: 0, max: 10, merge_enabled: true };
      }
      if (action === "message.list") {
        return {
          messages: [
            {
              id: "message-1",
              session_id: PRIMARY_SESSION_ID,
              task_id: "task-1",
              content: "ordinary\n\nresolved",
              metadata: {
                send_now_sources: [
                  { id: "ordinary", metadata: {} },
                  { id: CLIENT_QUEUE_ID, metadata: { client_queue_id: CLIENT_QUEUE_ID } },
                ],
              },
              created_at: "2026-09-02T00:00:00Z",
            },
          ],
        };
      }
      return undefined;
    });
    getWebSocketClientMock.mockReturnValue({ request });

    await expect(
      queueMessage({
        session_id: PRIMARY_SESSION_ID,
        session_incarnation_id: INCARNATION_ID,
        task_id: "task-1",
        client_queue_id: CLIENT_QUEUE_ID,
        content: "",
        plan_comment_refs: [{ id: "comment-1", version: 2 }],
      }),
    ).resolves.toMatchObject({ id: CLIENT_QUEUE_ID, content: "ordinary\n\nresolved" });
  });

  it("searches older transcript pages for an already-drained admission", async () => {
    const request = vi.fn(async (action: string, params?: { before?: string }) => {
      if (action === QUEUE_ADD_ACTION) throw new Error("WebSocket request timed out");
      if (action === "message.queue.get") {
        return { entries: [], count: 0, max: 10, merge_enabled: true };
      }
      if (action === "message.list" && !params?.before) {
        return { messages: [], has_more: true, cursor: "message-100" };
      }
      if (action === "message.list" && params?.before === "message-100") {
        return {
          messages: [
            {
              id: "message-older",
              session_id: PRIMARY_SESSION_ID,
              task_id: "task-1",
              content: "resolved earlier",
              metadata: { client_queue_id: CLIENT_QUEUE_ID },
              created_at: "2026-09-02T00:00:00Z",
            },
          ],
          has_more: false,
        };
      }
      return undefined;
    });
    getWebSocketClientMock.mockReturnValue({ request });

    await expect(
      queueMessage({
        session_id: PRIMARY_SESSION_ID,
        session_incarnation_id: INCARNATION_ID,
        task_id: "task-1",
        client_queue_id: CLIENT_QUEUE_ID,
        content: "",
        plan_comment_refs: [{ id: "comment-1", version: 2 }],
      }),
    ).resolves.toMatchObject({ id: CLIENT_QUEUE_ID, content: "resolved earlier" });
    expect(request).toHaveBeenCalledWith(
      "message.list",
      expect.objectContaining({ before: "message-100" }),
      5000,
    );
  });

  it("retries an uncertain admission after reconciliation finds no record", async () => {
    const queued = {
      id: CLIENT_QUEUE_ID,
      session_id: PRIMARY_SESSION_ID,
      task_id: "task-1",
      content: "resolved",
      queued_at: "2026-09-02T00:00:00Z",
    };
    let addCalls = 0;
    const request = vi.fn(async (action: string) => {
      if (action === QUEUE_ADD_ACTION) {
        addCalls++;
        if (addCalls === 1) throw new Error("WebSocket request timed out");
        return queued;
      }
      if (action === "message.queue.get") return { entries: [], count: 0, max: 10 };
      if (action === "message.list") return { messages: [], has_more: false };
      return undefined;
    });
    getWebSocketClientMock.mockReturnValue({ request });

    await expect(
      queueMessage({
        session_id: PRIMARY_SESSION_ID,
        session_incarnation_id: INCARNATION_ID,
        task_id: "task-1",
        client_queue_id: CLIENT_QUEUE_ID,
        content: "",
        plan_comment_refs: [{ id: "comment-1", version: 2 }],
      }),
    ).resolves.toEqual(queued);
    expect(addCalls).toBe(2);
  });

  it("reconciles acceptance after the uncertain retry also loses its response", async () => {
    const queued = {
      id: CLIENT_QUEUE_ID,
      session_id: PRIMARY_SESSION_ID,
      task_id: "task-1",
      content: "resolved",
      queued_at: "2026-09-02T00:00:00Z",
    };
    let queueReads = 0;
    const request = vi.fn(async (action: string) => {
      if (action === QUEUE_ADD_ACTION) throw new Error("WebSocket request timed out");
      if (action === "message.queue.get") {
        queueReads++;
        return queueReads === 1
          ? { entries: [], count: 0, max: 10 }
          : { entries: [queued], count: 1, max: 10 };
      }
      if (action === "message.list") return { messages: [], has_more: false };
      return undefined;
    });
    getWebSocketClientMock.mockReturnValue({ request });

    await expect(
      queueMessage({
        session_id: PRIMARY_SESSION_ID,
        session_incarnation_id: INCARNATION_ID,
        task_id: "task-1",
        client_queue_id: CLIENT_QUEUE_ID,
        content: "",
        plan_comment_refs: [{ id: "comment-1", version: 2 }],
      }),
    ).resolves.toEqual(queued);
    expect(request).toHaveBeenCalledTimes(5);
  });
});
