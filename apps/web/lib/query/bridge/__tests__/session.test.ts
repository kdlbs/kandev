import { describe, it, expect, beforeEach } from "vitest";
import { createTestQueryClient } from "@/test-utils/render-with-query";
import { registerSessionBridge } from "../session";
import { qk } from "@/lib/query/keys";
import type { MessagesData, TurnsData, TaskPlanData } from "@/lib/query/query-options/session";

// ---------------------------------------------------------------------------
// WS event name constants
// ---------------------------------------------------------------------------

const MSG_ADDED = "session.message.added";
const MSG_UPDATED = "session.message.updated";
const TURN_STARTED = "session.turn.started";
const TURN_COMPLETED = "session.turn.completed";

// ---------------------------------------------------------------------------
// Shared timestamps
// ---------------------------------------------------------------------------

const T0 = "2024-01-01T00:00:00Z";
const T1 = "2024-01-01T00:01:00Z";

// ---------------------------------------------------------------------------
// Fake WebSocket client
// ---------------------------------------------------------------------------
type Handler = (message: { payload: Record<string, unknown>; timestamp?: string }) => void;

function makeFakeWs() {
  const handlers = new Map<string, Set<Handler>>();
  return {
    on(event: string, handler: Handler) {
      let set = handlers.get(event);
      if (!set) {
        set = new Set();
        handlers.set(event, set);
      }
      set.add(handler);
      return () => set?.delete(handler);
    },
    emit(event: string, payload: Record<string, unknown>, timestamp?: string) {
      const set = handlers.get(event);
      if (!set) return;
      for (const h of set) h({ payload, timestamp });
    },
  };
}

// ---------------------------------------------------------------------------
// Shared setup
// ---------------------------------------------------------------------------

type TestContext = {
  ws: ReturnType<typeof makeFakeWs>;
  qc: ReturnType<typeof createTestQueryClient>;
  cleanup: () => void;
};

function makeContext(): TestContext {
  const ws = makeFakeWs();
  const qc = createTestQueryClient();
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const cleanup = registerSessionBridge(ws as any, qc);
  return { ws, qc, cleanup };
}

// ---------------------------------------------------------------------------
// Message tests
// ---------------------------------------------------------------------------

describe("session bridge — messages", () => {
  let ctx: TestContext;
  beforeEach(() => {
    ctx = makeContext();
  });

  it("seeds message cache on first session.message.added", () => {
    ctx.ws.emit(MSG_ADDED, {
      message_id: "msg-1",
      session_id: "sess-1",
      task_id: "task-1",
      author_type: "user",
      content: "hello",
      type: "message",
      created_at: T0,
    });

    const data = ctx.qc.getQueryData<MessagesData>(qk.session.messages("sess-1"));
    expect(data?.messages).toHaveLength(1);
    expect(data?.messages[0].id).toBe("msg-1");
    expect(data?.messages[0].content).toBe("hello");
  });

  it("deduplicates messages by id on session.message.added", () => {
    ctx.ws.emit(MSG_ADDED, {
      message_id: "msg-dup",
      session_id: "sess-2",
      task_id: "task-2",
      author_type: "agent",
      content: "first",
      type: "message",
      created_at: T0,
    });

    ctx.ws.emit(MSG_ADDED, {
      message_id: "msg-dup",
      session_id: "sess-2",
      task_id: "task-2",
      author_type: "agent",
      content: "updated content",
      type: "message",
      created_at: T0,
    });

    const data = ctx.qc.getQueryData<MessagesData>(qk.session.messages("sess-2"));
    expect(data?.messages).toHaveLength(1);
    expect(data?.messages[0].content).toBe("updated content");
  });

  it("updates existing message on session.message.updated", () => {
    ctx.ws.emit(MSG_ADDED, {
      message_id: "msg-upd",
      session_id: "sess-3",
      task_id: "task-3",
      author_type: "agent",
      content: "original",
      type: "tool_call",
      created_at: T0,
    });

    ctx.ws.emit(MSG_UPDATED, {
      message_id: "msg-upd",
      session_id: "sess-3",
      task_id: "task-3",
      author_type: "agent",
      content: "original",
      type: "tool_call",
      metadata: { status: "complete" },
      created_at: T0,
    });

    const data = ctx.qc.getQueryData<MessagesData>(qk.session.messages("sess-3"));
    expect(data?.messages[0].metadata).toMatchObject({ status: "complete" });
  });

  it("ignores message.updated when message not in cache", () => {
    ctx.ws.emit(MSG_UPDATED, {
      message_id: "msg-ghost",
      session_id: "sess-4",
      task_id: "task-4",
      author_type: "agent",
      content: "ghost",
      type: "message",
      created_at: T0,
    });

    expect(ctx.qc.getQueryData(qk.session.messages("sess-4"))).toBeUndefined();
  });

  it("cleanup removes message handlers", () => {
    ctx.cleanup();
    ctx.ws.emit(MSG_ADDED, {
      message_id: "post-cleanup",
      session_id: "sess-99",
      task_id: "task-99",
      author_type: "user",
      content: "should not appear",
      type: "message",
      created_at: T0,
    });
    expect(ctx.qc.getQueryData(qk.session.messages("sess-99"))).toBeUndefined();
  });
});

// ---------------------------------------------------------------------------
// Turn tests
// ---------------------------------------------------------------------------

describe("session bridge — turns", () => {
  let ctx: TestContext;
  beforeEach(() => {
    ctx = makeContext();
  });

  it("adds turn on session.turn.started", () => {
    ctx.ws.emit(TURN_STARTED, {
      id: "turn-1",
      session_id: "sess-5",
      task_id: "task-5",
      started_at: T0,
      created_at: T0,
      updated_at: T0,
    });

    const data = ctx.qc.getQueryData<TurnsData>(["session", "sess-5", "turns"]);
    expect(data?.turns).toHaveLength(1);
    expect(data?.activeTurnId).toBe("turn-1");
  });

  it("completes turn on session.turn.completed and clears activeTurnId", () => {
    ctx.ws.emit(TURN_STARTED, {
      id: "turn-2",
      session_id: "sess-6",
      task_id: "task-6",
      started_at: T0,
      created_at: T0,
      updated_at: T0,
    });

    ctx.ws.emit(TURN_COMPLETED, {
      id: "turn-2",
      session_id: "sess-6",
      task_id: "task-6",
      completed_at: T1,
    });

    const data = ctx.qc.getQueryData<TurnsData>(["session", "sess-6", "turns"]);
    expect(data?.activeTurnId).toBeNull();
    expect(data?.turns[0].completed_at).toBe(T1);
  });

  it("marks in-progress tool calls as complete on turn completion", () => {
    const initialData: MessagesData = {
      messages: [
        {
          id: "msg-tool",
          session_id: "sess-7" as import("@/lib/types/http").SessionId,
          task_id: "task-7" as import("@/lib/types/http").TaskId,
          author_type: "agent",
          content: "calling tool",
          type: "tool_call",
          metadata: { tool_call_id: "tc-1", status: "running" },
          created_at: T0,
        },
      ],
      hasMore: false,
      oldestCursor: "msg-tool",
    };
    ctx.qc.setQueryData(qk.session.messages("sess-7"), initialData);

    ctx.ws.emit(TURN_COMPLETED, {
      id: "turn-3",
      session_id: "sess-7",
      task_id: "task-7",
      completed_at: T1,
    });

    const data = ctx.qc.getQueryData<MessagesData>(qk.session.messages("sess-7"));
    expect((data?.messages[0].metadata as Record<string, unknown>)?.status).toBe("complete");
  });
});

// ---------------------------------------------------------------------------
// Task plan tests
// ---------------------------------------------------------------------------

describe("session bridge — task plans", () => {
  let ctx: TestContext;
  beforeEach(() => {
    ctx = makeContext();
  });

  it("stores task plan on task.plan.created", () => {
    ctx.ws.emit("task.plan.created", {
      id: "plan-1",
      task_id: "task-10",
      title: "My Plan",
      content: "# Plan\n",
      created_by: "agent",
      created_at: T0,
      updated_at: T0,
    });

    const data = ctx.qc.getQueryData<TaskPlanData>(["session", "plans", "task-10"]);
    expect(data?.plan?.title).toBe("My Plan");
  });

  it("updates task plan on task.plan.updated", () => {
    ctx.qc.setQueryData(["session", "plans", "task-11"] as const, {
      plan: {
        id: "plan-2",
        task_id: "task-11",
        title: "Old",
        content: "old",
        created_by: "agent",
        created_at: "t",
        updated_at: "t1",
      },
      lastSeenUpdatedAt: "t1",
    });

    ctx.ws.emit("task.plan.updated", {
      id: "plan-2",
      task_id: "task-11",
      title: "Updated",
      content: "new content",
      created_by: "agent",
      created_at: "t",
      updated_at: "t2",
    });

    const data = ctx.qc.getQueryData<TaskPlanData>(["session", "plans", "task-11"]);
    expect(data?.plan?.title).toBe("Updated");
  });

  it("nullifies plan on task.plan.deleted", () => {
    ctx.qc.setQueryData(["session", "plans", "task-12"] as const, {
      plan: {
        id: "plan-3",
        task_id: "task-12",
        title: "Plan",
        content: "x",
        created_by: "user",
        created_at: "t",
        updated_at: "t1",
      },
      lastSeenUpdatedAt: "t1",
    });

    ctx.ws.emit("task.plan.deleted", { task_id: "task-12" });

    const data = ctx.qc.getQueryData<TaskPlanData>(["session", "plans", "task-12"]);
    expect(data?.plan).toBeNull();
  });
});
