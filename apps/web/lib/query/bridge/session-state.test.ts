/**
 * Tests for the session-state WS → TQ bridge (D4 + D6 Stage 1).
 *
 * Verifies that session.state_changed and session.agentctl_* events mirror the
 * TaskSession record into BOTH the by-id and by-task TQ caches (mergeTaskSession
 * semantics) and drive the D6 side-effects (env mapping + prepare progress).
 */

import { describe, it, expect, vi } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import { registerSessionStateBridge, type SessionStateBridgeDeps } from "./session-state";
import { qk } from "@/lib/query/keys";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type TaskSession,
  type TaskSessionsResponse,
} from "@/lib/types/http";
import type { WebSocketClient } from "@/lib/ws/client";

type Handler = (msg: { payload: Record<string, unknown> }) => void;

function makeFakeWs() {
  const listeners = new Map<string, Set<Handler>>();
  return {
    on: vi.fn((type: string, handler: Handler) => {
      const set = listeners.get(type) ?? new Set<Handler>();
      set.add(handler);
      listeners.set(type, set);
      return () => listeners.get(type)?.delete(handler);
    }),
    emit(type: string, payload: Record<string, unknown>, timestamp?: string) {
      listeners.get(type)?.forEach((h) => h({ payload, ...(timestamp ? { timestamp } : {}) }));
    },
    listenerCount(type: string) {
      return listeners.get(type)?.size ?? 0;
    },
  };
}

function makeDeps(): SessionStateBridgeDeps & {
  envCalls: Array<[string, string]>;
} {
  const envCalls: Array<[string, string]> = [];
  return {
    envCalls,
    setEnvMapping: (sid, env) => envCalls.push([sid, env]),
  };
}

const TASK = "task-1";
const SID = "sess-1";
const STATE_CHANGED = "session.state_changed";
const AGENTCTL_STARTING = "session.agentctl_starting";
const AGENTCTL_READY = "session.agentctl_ready";
const AGENTCTL_ERROR = "session.agentctl_error";

function setup() {
  const ws = makeFakeWs();
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
  const deps = makeDeps();
  const cleanup = registerSessionStateBridge(ws as unknown as WebSocketClient, qc, deps);
  return { ws, qc, deps, cleanup };
}

describe("registerSessionStateBridge", () => {
  it("returns a cleanup function that unsubscribes all handlers", () => {
    const { ws, cleanup } = setup();
    expect(ws.listenerCount("session.state_changed")).toBe(1);
    expect(ws.listenerCount(AGENTCTL_STARTING)).toBe(1);
    expect(ws.listenerCount(AGENTCTL_READY)).toBe(1);
    expect(ws.listenerCount(AGENTCTL_ERROR)).toBe(1);
    cleanup();
    expect(ws.listenerCount("session.state_changed")).toBe(0);
    expect(ws.listenerCount(AGENTCTL_READY)).toBe(0);
  });

  it("session.state_changed writes the session into BOTH by-id and by-task caches", () => {
    const { ws, qc } = setup();

    ws.emit(STATE_CHANGED, {
      task_id: TASK,
      session_id: SID,
      new_state: "RUNNING",
      agent_profile_id: "profile-x",
    });

    const byId = qc.getQueryData<TaskSession | null>(qk.taskSession.byId(SID));
    expect(byId?.id).toBe(SID);
    expect(byId?.state).toBe("RUNNING");
    expect(byId?.agent_profile_id).toBe("profile-x");

    const byTask = qc.getQueryData<TaskSessionsResponse>(qk.taskSession.byTask(TASK));
    expect(byTask?.sessions).toHaveLength(1);
    expect(byTask?.sessions[0].state).toBe("RUNNING");
    expect(byTask?.total).toBe(1);
  });

  it("merges successive state_changed events without dropping prior fields (mergeTaskSession)", () => {
    const { ws, qc } = setup();

    ws.emit(STATE_CHANGED, {
      task_id: TASK,
      session_id: SID,
      new_state: "STARTING",
      agent_profile_id: "profile-x",
    });
    ws.emit(STATE_CHANGED, {
      task_id: TASK,
      session_id: SID,
      new_state: "RUNNING",
    });

    const byId = qc.getQueryData<TaskSession | null>(qk.taskSession.byId(SID));
    expect(byId?.state).toBe("RUNNING");
    // agent_profile_id from the first event must survive (merge, not replace)
    expect(byId?.agent_profile_id).toBe("profile-x");

    const byTask = qc.getQueryData<TaskSessionsResponse>(qk.taskSession.byTask(TASK));
    expect(byTask?.sessions).toHaveLength(1);
    expect(byTask?.sessions[0].agent_profile_id).toBe("profile-x");
  });

  it("ignores an unknown session whose state_changed payload has no new_state", () => {
    const { ws, qc } = setup();

    ws.emit(STATE_CHANGED, { task_id: TASK, session_id: SID, error_message: "boom" });

    expect(qc.getQueryData(qk.taskSession.byId(SID))).toBeUndefined();
    expect(qc.getQueryData(qk.taskSession.byTask(TASK))).toBeUndefined();
  });

  it("upserts into an existing by-task list rather than seeding a new one", () => {
    const { ws, qc } = setup();
    const other: TaskSession = {
      id: toSessionId("sess-other"),
      task_id: toTaskId(TASK),
      state: "RUNNING",
      started_at: "",
      updated_at: "",
    };
    qc.setQueryData<TaskSessionsResponse>(qk.taskSession.byTask(TASK), {
      sessions: [other],
      total: 1,
    });

    ws.emit(STATE_CHANGED, { task_id: TASK, session_id: SID, new_state: "STARTING" });

    const byTask = qc.getQueryData<TaskSessionsResponse>(qk.taskSession.byTask(TASK));
    expect(byTask?.sessions).toHaveLength(2);
    expect(byTask?.total).toBe(2);
    expect(byTask?.sessions.map((s) => s.id)).toContain(SID);
    expect(byTask?.sessions.map((s) => s.id)).toContain("sess-other");
  });

  it("session.state_changed with task_environment_id drives the env-mapping side effect", () => {
    const { ws, deps } = setup();

    ws.emit(STATE_CHANGED, {
      task_id: TASK,
      session_id: SID,
      new_state: "RUNNING",
      task_environment_id: "env-99",
    });

    expect(deps.envCalls).toContainEqual([SID, "env-99"]);
  });
});

describe("registerSessionStateBridge — agentctl + prepare", () => {
  it("session.agentctl_ready merges worktree fields + env mapping into the by-id cache", () => {
    const { ws, qc, deps } = setup();
    // Seed an existing record so agentctl_ready merges into it.
    qc.setQueryData<TaskSession>(qk.taskSession.byId(SID), {
      id: toSessionId(SID),
      task_id: toTaskId(TASK),
      state: "RUNNING",
      started_at: "",
      updated_at: "",
    });

    ws.emit(AGENTCTL_READY, {
      task_id: TASK,
      session_id: SID,
      task_environment_id: "env-7",
      worktree_id: "wt-1",
      worktree_path: "/work/tree",
      worktree_branch: "feature/x",
    });

    const byId = qc.getQueryData<TaskSession | null>(qk.taskSession.byId(SID));
    expect(byId?.state).toBe("RUNNING"); // preserved
    expect(byId?.worktree_id).toBe("wt-1");
    expect(byId?.worktree_path).toBe("/work/tree");
    expect(byId?.worktree_branch).toBe("feature/x");
    expect(byId?.task_environment_id).toBe("env-7");
    expect(deps.envCalls).toContainEqual([SID, "env-7"]);
  });

  it("session.agentctl_starting seeds a CREATED record + env mapping for a brand-new session", () => {
    const { ws, qc, deps } = setup();

    ws.emit(AGENTCTL_STARTING, {
      task_id: TASK,
      session_id: SID,
      task_environment_id: "env-3",
    });

    const byId = qc.getQueryData<TaskSession | null>(qk.taskSession.byId(SID));
    expect(byId?.id).toBe(SID);
    expect(byId?.state).toBe("CREATED");
    expect(byId?.task_environment_id).toBe("env-3");
    expect(deps.envCalls).toContainEqual([SID, "env-3"]);
  });

  it("writes the agentctl status badge (starting/ready/error) into qk.session.agentctl", () => {
    const { ws, qc } = setup();

    ws.emit(
      AGENTCTL_STARTING,
      { task_id: TASK, session_id: SID, agent_execution_id: "ae-1" },
      "2026-05-31T00:00:00Z",
    );
    expect(qc.getQueryData(qk.session.agentctl(SID))).toMatchObject({
      status: "starting",
      agentExecutionId: "ae-1",
      updatedAt: "2026-05-31T00:00:00Z",
    });

    ws.emit(AGENTCTL_READY, {
      task_id: TASK,
      session_id: SID,
      agent_execution_id: "ae-1",
    });
    expect(qc.getQueryData(qk.session.agentctl(SID))).toMatchObject({ status: "ready" });

    ws.emit(AGENTCTL_ERROR, {
      task_id: TASK,
      session_id: SID,
      error_message: "container crashed",
    });
    expect(qc.getQueryData(qk.session.agentctl(SID))).toMatchObject({
      status: "error",
      errorMessage: "container crashed",
    });
  });

  it("writes the agentctl status even when the event carries no task_id (snapshot replay)", () => {
    const { ws, qc } = setup();

    // No cached session + no task_id: the TaskSession merge is skipped, but the
    // status badge (keyed purely on session_id) must still land.
    ws.emit(AGENTCTL_READY, { session_id: SID });

    expect(qc.getQueryData(qk.session.agentctl(SID))).toMatchObject({ status: "ready" });
    expect(qc.getQueryData(qk.taskSession.byId(SID))).toBeUndefined();
  });

  it("mirrors metadata.prepare_result into the prepare-progress TQ cache", () => {
    const { ws, qc } = setup();

    ws.emit(STATE_CHANGED, {
      task_id: TASK,
      session_id: SID,
      new_state: "RUNNING",
      session_metadata: {
        prepare_result: {
          status: "completed",
          steps: [{ name: "clone", status: "completed" }],
        },
      },
    });

    const prepare = qc.getQueryData(qk.session.prepareProgress(SID));
    expect(prepare).toMatchObject({ sessionId: SID, status: "completed" });
  });
});

// Out-of-order subscribe-snapshot guard (fix #1208): a state_changed carrying
// an older updated_at than the record we already hold must not stomp the
// fresher state (which would revert a live WAITING_FOR_INPUT to STARTING and
// block idle input).
describe("registerSessionStateBridge — stale snapshot guard", () => {
  const OLDER = "2026-01-01T00:00:00.000Z";
  const NEWER = "2026-01-02T00:00:00.000Z";

  it("records updated_at so later out-of-order snapshots can be dropped", () => {
    const { ws, qc } = setup();

    ws.emit(STATE_CHANGED, {
      task_id: TASK,
      session_id: SID,
      new_state: "WAITING_FOR_INPUT",
      updated_at: NEWER,
    });

    const byId = qc.getQueryData<TaskSession | null>(qk.taskSession.byId(SID));
    expect(byId?.updated_at).toBe(NEWER);
  });

  it("drops a stale snapshot (older updated_at) instead of stomping fresher state", () => {
    const { ws, qc } = setup();

    // Live state lands first.
    ws.emit(STATE_CHANGED, {
      task_id: TASK,
      session_id: SID,
      new_state: "WAITING_FOR_INPUT",
      updated_at: NEWER,
    });
    // A subscribe snapshot read before the live state arrives later, out of
    // order, carrying an older updated_at — it must be ignored.
    ws.emit(STATE_CHANGED, {
      task_id: TASK,
      session_id: SID,
      new_state: "STARTING",
      updated_at: OLDER,
    });

    const byId = qc.getQueryData<TaskSession | null>(qk.taskSession.byId(SID));
    expect(byId?.state).toBe("WAITING_FOR_INPUT");
    expect(byId?.updated_at).toBe(NEWER);
  });

  it("applies a newer snapshot (later updated_at)", () => {
    const { ws, qc } = setup();

    ws.emit(STATE_CHANGED, {
      task_id: TASK,
      session_id: SID,
      new_state: "STARTING",
      updated_at: OLDER,
    });
    ws.emit(STATE_CHANGED, {
      task_id: TASK,
      session_id: SID,
      new_state: "RUNNING",
      updated_at: NEWER,
    });

    const byId = qc.getQueryData<TaskSession | null>(qk.taskSession.byId(SID));
    expect(byId?.state).toBe("RUNNING");
    expect(byId?.updated_at).toBe(NEWER);
  });
});
