import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionSlice } from "./session-slice";
import { createSessionRuntimeSlice } from "../session-runtime/session-runtime-slice";
import type { SessionSlice } from "./types";
import type { MCPAttachmentHistory, SessionRuntimeSlice } from "../session-runtime/types";
import { sessionId as toSessionId, taskId as toTaskId, type TaskSession } from "@/lib/types/http";

type CombinedSlice = SessionSlice & SessionRuntimeSlice;

function makeStore() {
  return create<CombinedSlice>()(
    immer((set) => ({
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...(createSessionSlice as any)(set),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...(createSessionRuntimeSlice as any)(set),
    })),
  );
}

const TASK_ID = toTaskId("task-1");
const SESSION_ID = toSessionId("session-1");
const SIBLING_SESSION_ID = toSessionId("session-2");
const SESSION_TIMESTAMP = "2026-04-20T00:00:00Z";
const MIDDLE_TIMESTAMP = "2026-04-20T00:01:00Z";
const LATEST_TIMESTAMP = "2026-04-20T00:02:00Z";

function makeSession(id: string, metadata?: Record<string, unknown>): TaskSession {
  return {
    id: toSessionId(id),
    task_id: TASK_ID,
    state: "RUNNING",
    started_at: SESSION_TIMESTAMP,
    updated_at: SESSION_TIMESTAMP,
    ...(metadata ? { metadata } : {}),
  };
}

function makeHistory({
  attemptId = "attempt-persisted",
  startedAt = "2026-04-20T00:00:00Z",
  updatedAt,
}: {
  attemptId?: string;
  startedAt?: string;
  updatedAt?: string;
} = {}): MCPAttachmentHistory {
  const current: MCPAttachmentHistory["current"] = {
    attachment_attempt_id: attemptId,
    started_at: startedAt,
    servers: [{ name: "kandev", status: "active" }],
  };
  if (updatedAt !== undefined) current.updated_at = updatedAt;
  return { version: 1, current };
}

function withCurrentPatch(patch: Record<string, unknown>): Record<string, unknown> {
  const history = makeHistory();
  return { ...history, current: { ...history.current, ...patch } };
}

describe("setTaskSessionsForTask MCP history", () => {
  it.each([
    ["started_at", makeHistory()],
    [
      "updated_at",
      makeHistory({
        startedAt: MIDDLE_TIMESTAMP,
        updatedAt: LATEST_TIMESTAMP,
      }),
    ],
  ])("restores valid attachment history using %s freshness", (_freshness, history) => {
    const store = makeStore();

    store
      .getState()
      .setTaskSessionsForTask(
        TASK_ID,
        [makeSession("session-1", { mcp_attachment_state: history })],
        {},
      );

    expect(store.getState().sessionMcpStatus.bySessionId[SESSION_ID]).toEqual(history);
  });
});

describe("setTaskSessionsForTask MCP history freshness", () => {
  it.each([
    ["older", makeHistory({ attemptId: "attempt-older", startedAt: MIDDLE_TIMESTAMP })],
    [
      "equal",
      makeHistory({
        attemptId: "attempt-equal",
        startedAt: LATEST_TIMESTAMP,
        updatedAt: LATEST_TIMESTAMP,
      }),
    ],
  ])("keeps newer live evidence when the incoming snapshot is %s", (_age, incoming) => {
    const store = makeStore();
    const live = makeHistory({
      attemptId: "attempt-live",
      startedAt: LATEST_TIMESTAMP,
      updatedAt: LATEST_TIMESTAMP,
    });
    store.getState().setSessionMCPStatus(SESSION_ID, live);

    store
      .getState()
      .setTaskSessionsForTask(
        TASK_ID,
        [makeSession("session-1", { mcp_attachment_state: incoming })],
        {},
      );

    expect(store.getState().sessionMcpStatus.bySessionId[SESSION_ID]).toEqual(live);
  });

  it("replaces stored history when its freshness timestamp is invalid", () => {
    const store = makeStore();
    const stored = makeHistory({ attemptId: "attempt-invalid", startedAt: "not-a-timestamp" });
    const incoming = makeHistory({ attemptId: "attempt-valid", startedAt: MIDDLE_TIMESTAMP });
    store.getState().setSessionMCPStatus(SESSION_ID, stored);

    store
      .getState()
      .setTaskSessionsForTask(
        TASK_ID,
        [makeSession("session-1", { mcp_attachment_state: incoming })],
        {},
      );

    expect(store.getState().sessionMcpStatus.bySessionId[SESSION_ID]).toEqual(incoming);
  });
});

describe("setTaskSessionsForTask MCP history isolation", () => {
  it("updates only the session that owns restored history", () => {
    const store = makeStore();
    const ownerHistory = makeHistory({
      attemptId: "attempt-owner",
      startedAt: MIDDLE_TIMESTAMP,
    });
    const siblingHistory = makeHistory({
      attemptId: "attempt-sibling",
      startedAt: LATEST_TIMESTAMP,
    });
    store.getState().setSessionMCPStatus(SIBLING_SESSION_ID, siblingHistory);

    store
      .getState()
      .setTaskSessionsForTask(
        TASK_ID,
        [
          makeSession("session-1", { mcp_attachment_state: ownerHistory }),
          makeSession("session-2"),
        ],
        {},
      );

    expect(store.getState().sessionMcpStatus.bySessionId[SESSION_ID]).toEqual(ownerHistory);
    expect(store.getState().sessionMcpStatus.bySessionId[SIBLING_SESSION_ID]).toEqual(
      siblingHistory,
    );
  });

  it("accepts additive fields in a valid history", () => {
    const store = makeStore();
    const base = makeHistory();
    const history = {
      ...base,
      report_version: "backend-1",
      current: {
        ...base.current,
        execution_id: "execution-1",
        servers: [{ ...base.current.servers![0], server_revision: 3 }],
      },
    };

    store
      .getState()
      .setTaskSessionsForTask(
        TASK_ID,
        [makeSession("session-1", { mcp_attachment_state: history })],
        {},
      );

    expect(store.getState().sessionMcpStatus.bySessionId[SESSION_ID]).toEqual(history);
  });
});

describe("setTaskSessionsForTask MCP history validation", () => {
  it.each([
    ["unsupported version", { ...makeHistory(), version: 2 }],
    ["missing current attempt", { version: 1 }],
    ["empty attempt identity", withCurrentPatch({ attachment_attempt_id: " " })],
    ["invalid started_at", withCurrentPatch({ started_at: "2026-02-30T00:00:00Z" })],
    ["invalid updated_at", withCurrentPatch({ updated_at: "tomorrow" })],
    [
      "invalid server timestamp",
      withCurrentPatch({
        servers: [{ name: "kandev", status: "active", tools_listed_at: "later" }],
      }),
    ],
    ["empty server name", withCurrentPatch({ servers: [{ name: " ", status: "active" }] })],
    [
      "unknown server status",
      withCurrentPatch({ servers: [{ name: "kandev", status: "online" }] }),
    ],
    ["non-array previous attempts", { ...makeHistory(), previous: {} }],
    [
      "invalid previous attempt",
      { ...makeHistory(), previous: [{ ...makeHistory().current, attachment_attempt_id: "" }] },
    ],
  ])("ignores %s metadata and preserves existing statuses", (_reason, invalidHistory) => {
    const store = makeStore();
    const ownerHistory = makeHistory({ attemptId: "attempt-owner" });
    const siblingHistory = makeHistory({ attemptId: "attempt-sibling" });
    store.getState().setSessionMCPStatus(SESSION_ID, ownerHistory);
    store.getState().setSessionMCPStatus(SIBLING_SESSION_ID, siblingHistory);

    store
      .getState()
      .setTaskSessionsForTask(
        TASK_ID,
        [
          makeSession("session-1", { mcp_attachment_state: invalidHistory }),
          makeSession("session-2"),
        ],
        {},
      );

    expect(store.getState().sessionMcpStatus.bySessionId[SESSION_ID]).toEqual(ownerHistory);
    expect(store.getState().sessionMcpStatus.bySessionId[SIBLING_SESSION_ID]).toEqual(
      siblingHistory,
    );
  });
});
