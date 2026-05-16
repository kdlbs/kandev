import { describe, it, expect } from "vitest";
import {
  selectActiveSessionsForAgent,
  selectAllSessionsForTask,
  selectCommandCount,
  selectLiveSessionForTask,
  selectSessionsByAgentForTask,
  selectTotalLiveSessions,
} from "./selectors";
import type { AppState } from "@/lib/state/store";
import {
  agentProfileId as toAgentProfileId,
  sessionId as toSessionId,
  taskId as toTaskId,
  type Message,
  type TaskSession,
  type TaskSessionState,
} from "@/lib/types/http";

const AGENT_A = "agent-a";
const AGENT_B = "agent-b";
const T_MAY_01 = "2026-05-01T10:00:00Z";
const T_MAY_02 = "2026-05-02T10:00:00Z";
const T_MAY_03 = "2026-05-03T10:00:00Z";

function makeSession(
  id: string,
  agentProfileId: string | undefined,
  state: TaskSessionState,
  overrides: Partial<TaskSession> = {},
): TaskSession {
  return {
    id: toSessionId(id),
    task_id: overrides.task_id ?? toTaskId(`task-${id}`),
    agent_profile_id: agentProfileId ? toAgentProfileId(agentProfileId) : undefined,
    state,
    started_at: overrides.started_at ?? "2026-05-03T00:00:00Z",
    updated_at: overrides.updated_at ?? "2026-05-03T00:00:00Z",
    ...overrides,
  };
}

function stateWithSessions(sessions: TaskSession[]): AppState {
  const items: Record<string, TaskSession> = {};
  for (const s of sessions) items[s.id] = s;
  return { taskSessions: { items }, messages: { bySession: {} } } as unknown as AppState;
}

function makeMessage(id: string, sessionId: string, type: Message["type"]): Message {
  return {
    id,
    session_id: toSessionId(sessionId),
    task_id: toTaskId("task-1"),
    author_type: "agent",
    content: "",
    type,
    created_at: "2026-05-03T00:00:00Z",
  };
}

describe("selectActiveSessionsForAgent", () => {
  it("returns 0 when no sessions exist for the agent", () => {
    const state = stateWithSessions([]);
    expect(selectActiveSessionsForAgent(state, AGENT_A)).toBe(0);
  });

  it("returns 0 when agentProfileId is empty", () => {
    const state = stateWithSessions([makeSession("s1", AGENT_A, "RUNNING")]);
    expect(selectActiveSessionsForAgent(state, "")).toBe(0);
  });

  it("counts RUNNING sessions for the agent only", () => {
    const state = stateWithSessions([
      makeSession("s1", AGENT_A, "RUNNING"),
      makeSession("s2", AGENT_A, "RUNNING"),
      makeSession("s3", AGENT_B, "RUNNING"),
      makeSession("s4", undefined, "RUNNING"),
    ]);
    expect(selectActiveSessionsForAgent(state, AGENT_A)).toBe(2);
    expect(selectActiveSessionsForAgent(state, AGENT_B)).toBe(1);
  });

  it("does NOT count WAITING_FOR_INPUT for office sessions (agent_profile_id set)", () => {
    // Office sessions skip WAITING_FOR_INPUT entirely — between turns they
    // sit in IDLE, which is not live. WAITING_FOR_INPUT on an office row
    // shouldn't happen, but if it did we treat it as not-live.
    const state = stateWithSessions([
      makeSession("s1", AGENT_A, "WAITING_FOR_INPUT"),
      makeSession("s2", AGENT_A, "RUNNING"),
    ]);
    expect(selectActiveSessionsForAgent(state, AGENT_A)).toBe(1);
  });

  it("does not count IDLE as active for office sessions", () => {
    const state = stateWithSessions([
      makeSession("s1", AGENT_A, "IDLE"),
      makeSession("s2", AGENT_A, "RUNNING"),
    ]);
    expect(selectActiveSessionsForAgent(state, AGENT_A)).toBe(1);
  });

  it("does not count COMPLETED, FAILED, CANCELLED, CREATED, STARTING, IDLE", () => {
    const state = stateWithSessions([
      makeSession("s1", AGENT_A, "COMPLETED"),
      makeSession("s2", AGENT_A, "FAILED"),
      makeSession("s3", AGENT_A, "CANCELLED"),
      makeSession("s4", AGENT_A, "CREATED"),
      makeSession("s5", AGENT_A, "STARTING"),
      makeSession("s6", AGENT_A, "IDLE"),
    ]);
    expect(selectActiveSessionsForAgent(state, AGENT_A)).toBe(0);
  });

  it("ignores sessions with no agent_profile_id", () => {
    const state = stateWithSessions([
      makeSession("s1", undefined, "RUNNING"),
      makeSession("s2", AGENT_A, "RUNNING"),
    ]);
    expect(selectActiveSessionsForAgent(state, AGENT_A)).toBe(1);
  });
});

describe("selectLiveSessionForTask", () => {
  it("returns null when taskId is empty", () => {
    const state = stateWithSessions([
      makeSession("s1", AGENT_A, "RUNNING", { task_id: toTaskId("task-1") }),
    ]);
    expect(selectLiveSessionForTask(state, "")).toBeNull();
  });

  it("returns null when no sessions match the task", () => {
    const state = stateWithSessions([
      makeSession("s1", AGENT_A, "RUNNING", { task_id: toTaskId("task-other") }),
    ]);
    expect(selectLiveSessionForTask(state, "task-1")).toBeNull();
  });

  it("returns null when sessions exist but none are live", () => {
    const state = stateWithSessions([
      makeSession("s1", AGENT_A, "COMPLETED", { task_id: toTaskId("task-1") }),
      makeSession("s2", AGENT_A, "FAILED", { task_id: toTaskId("task-1") }),
    ]);
    expect(selectLiveSessionForTask(state, "task-1")).toBeNull();
  });

  it("returns the only RUNNING session for the task", () => {
    const live = makeSession("s1", AGENT_A, "RUNNING", { task_id: toTaskId("task-1") });
    const state = stateWithSessions([live]);
    expect(selectLiveSessionForTask(state, "task-1")?.id).toBe("s1");
  });

  it("treats WAITING_FOR_INPUT as live for kanban (no agent_profile_id)", () => {
    const live = makeSession("s1", undefined, "WAITING_FOR_INPUT", { task_id: toTaskId("task-1") });
    const state = stateWithSessions([live]);
    expect(selectLiveSessionForTask(state, "task-1")?.id).toBe("s1");
  });

  it("does NOT treat IDLE as live for office sessions", () => {
    const idle = makeSession("s1", AGENT_A, "IDLE", { task_id: toTaskId("task-1") });
    const state = stateWithSessions([idle]);
    expect(selectLiveSessionForTask(state, "task-1")).toBeNull();
  });

  it("topbar drops the moment a turn ends: RUNNING → IDLE", () => {
    const before = stateWithSessions([
      makeSession("s1", AGENT_A, "RUNNING", { task_id: toTaskId("task-1") }),
    ]);
    expect(selectLiveSessionForTask(before, "task-1")?.id).toBe("s1");
    const after = stateWithSessions([
      makeSession("s1", AGENT_A, "IDLE", { task_id: toTaskId("task-1") }),
    ]);
    expect(selectLiveSessionForTask(after, "task-1")).toBeNull();
  });

  it("multi-agent task: returns the agent currently RUNNING, ignoring IDLE peers", () => {
    const state = stateWithSessions([
      makeSession("s1", AGENT_A, "IDLE", {
        task_id: toTaskId("task-1"),
        started_at: "2026-05-03T11:00:00Z",
      }),
      makeSession("s2", AGENT_B, "RUNNING", {
        task_id: toTaskId("task-1"),
        started_at: T_MAY_03,
      }),
    ]);
    expect(selectLiveSessionForTask(state, "task-1")?.id).toBe("s2");
  });

  it("picks the most recently started live session", () => {
    const state = stateWithSessions([
      makeSession("s1", AGENT_A, "RUNNING", {
        task_id: toTaskId("task-1"),
        started_at: T_MAY_01,
      }),
      makeSession("s2", AGENT_A, "RUNNING", {
        task_id: toTaskId("task-1"),
        started_at: T_MAY_03,
      }),
      makeSession("s3", AGENT_A, "RUNNING", {
        task_id: toTaskId("task-1"),
        started_at: T_MAY_02,
      }),
    ]);
    expect(selectLiveSessionForTask(state, "task-1")?.id).toBe("s2");
  });

  it("ignores sessions for other tasks", () => {
    const state = stateWithSessions([
      makeSession("s1", AGENT_A, "RUNNING", {
        task_id: toTaskId("task-other"),
        started_at: "2026-05-04T10:00:00Z",
      }),
      makeSession("s2", AGENT_A, "RUNNING", {
        task_id: toTaskId("task-1"),
        started_at: T_MAY_01,
      }),
    ]);
    expect(selectLiveSessionForTask(state, "task-1")?.id).toBe("s2");
  });
});

describe("selectAllSessionsForTask", () => {
  it("returns empty array when taskId is empty", () => {
    const state = stateWithSessions([
      makeSession("s1", AGENT_A, "RUNNING", { task_id: toTaskId("task-1") }),
    ]);
    expect(selectAllSessionsForTask(state, "")).toEqual([]);
  });

  it("returns sessions sorted by started_at ascending", () => {
    const state = stateWithSessions([
      makeSession("s1", AGENT_A, "COMPLETED", {
        task_id: toTaskId("task-1"),
        started_at: T_MAY_03,
      }),
      makeSession("s2", AGENT_A, "RUNNING", {
        task_id: toTaskId("task-1"),
        started_at: T_MAY_01,
      }),
      makeSession("s3", AGENT_A, "FAILED", {
        task_id: toTaskId("task-1"),
        started_at: T_MAY_02,
      }),
    ]);
    const result = selectAllSessionsForTask(state, "task-1");
    expect(result.map((s) => s.id)).toEqual(["s2", "s3", "s1"]);
  });

  it("ignores sessions for other tasks", () => {
    const state = stateWithSessions([
      makeSession("s1", AGENT_A, "RUNNING", { task_id: toTaskId("task-1") }),
      makeSession("s2", AGENT_A, "RUNNING", { task_id: toTaskId("task-2") }),
    ]);
    const result = selectAllSessionsForTask(state, "task-1");
    expect(result.map((s) => s.id)).toEqual(["s1"]);
  });
});

describe("selectTotalLiveSessions", () => {
  it("returns 0 when no sessions exist", () => {
    expect(selectTotalLiveSessions(stateWithSessions([]))).toBe(0);
  });

  it("counts RUNNING for office and (RUNNING | WAITING_FOR_INPUT) for kanban", () => {
    const state = stateWithSessions([
      makeSession("s1", AGENT_A, "RUNNING", { task_id: toTaskId("t-1") }), // office, live
      makeSession("s2", AGENT_B, "WAITING_FOR_INPUT", { task_id: toTaskId("t-2") }), // office, NOT live
      makeSession("s3", AGENT_A, "RUNNING", { task_id: toTaskId("t-3") }), // office, live
      makeSession("s4", AGENT_A, "COMPLETED", { task_id: toTaskId("t-4") }), // not live
      makeSession("s5", undefined, "WAITING_FOR_INPUT", { task_id: toTaskId("t-5") }), // kanban, live
    ]);
    expect(selectTotalLiveSessions(state)).toBe(3);
  });

  it("ignores all terminal/initial states (incl. IDLE)", () => {
    const state = stateWithSessions([
      makeSession("s1", AGENT_A, "COMPLETED"),
      makeSession("s2", AGENT_A, "FAILED"),
      makeSession("s3", AGENT_A, "CANCELLED"),
      makeSession("s4", AGENT_A, "CREATED"),
      makeSession("s5", AGENT_A, "STARTING"),
      makeSession("s6", AGENT_A, "IDLE"),
    ]);
    expect(selectTotalLiveSessions(state)).toBe(0);
  });
});

describe("selectSessionsByAgentForTask", () => {
  it("returns an empty Map when taskId is empty", () => {
    const state = stateWithSessions([
      makeSession("s1", AGENT_A, "RUNNING", { task_id: toTaskId("task-1") }),
    ]);
    expect(selectSessionsByAgentForTask(state, "").size).toBe(0);
  });

  it("groups sessions by agent_profile_id", () => {
    const state = stateWithSessions([
      makeSession("s1", AGENT_A, "RUNNING", { task_id: toTaskId("task-1") }),
      makeSession("s2", AGENT_B, "IDLE", { task_id: toTaskId("task-1") }),
      makeSession("s3", AGENT_A, "COMPLETED", { task_id: toTaskId("task-1") }),
    ]);
    const result = selectSessionsByAgentForTask(state, "task-1");
    expect(result.size).toBe(2);
    expect(
      result
        .get(AGENT_A)
        ?.map((s) => s.id)
        .sort(),
    ).toEqual(["s1", "s3"]);
    expect(result.get(AGENT_B)?.map((s) => s.id)).toEqual(["s2"]);
  });

  it("buckets kanban sessions (no agent_profile_id) under empty key", () => {
    const state = stateWithSessions([
      makeSession("s1", undefined, "RUNNING", { task_id: toTaskId("task-1") }),
      makeSession("s2", AGENT_A, "IDLE", { task_id: toTaskId("task-1") }),
    ]);
    const result = selectSessionsByAgentForTask(state, "task-1");
    expect(result.get("")?.map((s) => s.id)).toEqual(["s1"]);
    expect(result.get(AGENT_A)?.map((s) => s.id)).toEqual(["s2"]);
  });

  it("ignores sessions for other tasks", () => {
    const state = stateWithSessions([
      makeSession("s1", AGENT_A, "RUNNING", { task_id: toTaskId("task-1") }),
      makeSession("s2", AGENT_A, "RUNNING", { task_id: toTaskId("task-other") }),
    ]);
    const result = selectSessionsByAgentForTask(state, "task-1");
    expect(result.get(AGENT_A)?.map((s) => s.id)).toEqual(["s1"]);
  });

  it("orders agent buckets by most-recent activity (latest updated_at desc)", () => {
    const state = stateWithSessions([
      makeSession("s1", AGENT_A, "IDLE", {
        task_id: toTaskId("task-1"),
        started_at: T_MAY_01,
        updated_at: T_MAY_01,
      }),
      makeSession("s2", AGENT_B, "RUNNING", {
        task_id: toTaskId("task-1"),
        started_at: T_MAY_02,
        updated_at: T_MAY_03,
      }),
    ]);
    const result = selectSessionsByAgentForTask(state, "task-1");
    const keys = Array.from(result.keys());
    // AGENT_B has the more recent updated_at, so it comes first.
    expect(keys).toEqual([AGENT_B, AGENT_A]);
  });

  it("sorts within an agent bucket by started_at ascending", () => {
    const state = stateWithSessions([
      makeSession("s1", AGENT_A, "COMPLETED", {
        task_id: toTaskId("task-1"),
        started_at: T_MAY_03,
      }),
      makeSession("s2", AGENT_A, "IDLE", {
        task_id: toTaskId("task-1"),
        started_at: T_MAY_01,
      }),
    ]);
    const result = selectSessionsByAgentForTask(state, "task-1");
    expect(result.get(AGENT_A)?.map((s) => s.id)).toEqual(["s2", "s1"]);
  });
});

describe("selectCommandCount", () => {
  function stateWithMessages(bySession: Record<string, Message[]>): AppState {
    return {
      taskSessions: { items: {} },
      messages: { bySession },
    } as unknown as AppState;
  }

  it("returns 0 when sessionId is empty", () => {
    expect(selectCommandCount(stateWithMessages({}), "")).toBe(0);
  });

  it("returns 0 when there are no messages for the session", () => {
    expect(selectCommandCount(stateWithMessages({}), "s1")).toBe(0);
  });

  it("counts only messages with type 'tool_call'", () => {
    const state = stateWithMessages({
      s1: [
        makeMessage("m1", "s1", "tool_call"),
        makeMessage("m2", "s1", "message"),
        makeMessage("m3", "s1", "tool_call"),
        makeMessage("m4", "s1", "thinking"),
        makeMessage("m5", "s1", "tool_edit"),
      ],
    });
    expect(selectCommandCount(state, "s1")).toBe(2);
  });

  it("does not bleed across sessions", () => {
    const state = stateWithMessages({
      s1: [makeMessage("m1", "s1", "tool_call")],
      s2: [makeMessage("m2", "s2", "tool_call"), makeMessage("m3", "s2", "tool_call")],
    });
    expect(selectCommandCount(state, "s1")).toBe(1);
    expect(selectCommandCount(state, "s2")).toBe(2);
  });
});
