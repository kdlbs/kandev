import { describe, expect, it } from "vitest";
import type { NativeDashboard, NativeTask } from "@/lib/types/control-native";
import { buildAttention, countAttention } from "./control-attention";
import {
  capacityWaitingTotal,
  sortBySignal,
  taskSignal,
  workspaceFigures,
  workspaceStatus,
  type WorkspaceFigures,
} from "./control-signals";

const figures = (over: Partial<WorkspaceFigures> = {}): WorkspaceFigures => ({
  inProgress: 0,
  running: 0,
  waitingInput: 0,
  queued: 0,
  completed24h: 0,
  moves24h: 0,
  failedSessions: 0,
  ...over,
});

const task = (over: Partial<NativeTask> = {}): NativeTask => ({
  task_id: "t1",
  workspace_id: "w1",
  title: "Task",
  state: "IN_PROGRESS",
  step_id: "s",
  step_name: "Step",
  stage_type: "custom",
  session_id: "session-1",
  session_state: "WAITING_FOR_INPUT",
  archived: false,
  running_sessions: 0,
  queued_messages: 0,
  updated_at: "2026-10-19T10:00:00Z",
  ...over,
});

const base = {
  workspaceIds: ["w1"],
  figures: {},
  waiting: 0,
  tasks: [],
  repeats: new Map(),
  coordinators: { enabled: false, known: true, total: 0 },
};

describe("workspace figures and status", () => {
  it("counts in-progress states and reads sessions from the recorded dashboard", () => {
    const data = {
      tasks: { IN_PROGRESS: 2, REVIEW: 3, SCHEDULING: 1, TODO: 4 },
      sessions: { RUNNING: 1, WAITING_FOR_INPUT: 5, FAILED: 2 },
      waiting_input_tasks: 2,
      queued_messages: 3,
      transitions_24h: 9,
      completed_tasks_24h: 1,
    } as unknown as NativeDashboard;
    expect(workspaceFigures(data)).toEqual({
      inProgress: 6,
      running: 1,
      waitingInput: 2,
      queued: 3,
      completed24h: 1,
      moves24h: 9,
      failedSessions: 2,
    });
  });
  it("does not infer a stall or attribute shared capacity from recorded WIP alone", () => {
    const stuck = figures({ inProgress: 3 });
    expect(workspaceStatus(stuck, 0)).toBe("pending");
    expect(workspaceStatus(stuck, 2)).toBe("pending");
    expect(workspaceStatus(stuck, null)).toBe("unknown");
    expect(workspaceStatus(figures({ inProgress: 3, running: 1 }), 0)).toBe("active");
    expect(workspaceStatus(figures(), 0)).toBe("idle");
  });
  it("sums capacity waits and keeps an incomplete report unknown", () => {
    const observation = (known: boolean, counts?: Record<string, number>) =>
      ({ waiting: { known, counts } }) as never;
    expect(capacityWaitingTotal(observation(true, { a: 1, b: 2 }))).toBe(3);
    expect(capacityWaitingTotal(observation(false, { a: 0 }))).toBeNull();
    expect(capacityWaitingTotal(observation(true))).toBeNull();
    expect(capacityWaitingTotal(undefined)).toBeNull();
  });
});

describe("task signals", () => {
  const none = new Set<string>();
  it("preserves repeat and failure signals without treating queued messages as a stall", () => {
    expect(taskSignal(task({ queued_messages: 1 }), new Set(["t1"]))).toBe("repeat");
    expect(taskSignal(task({ queued_messages: 1, session_state: "FAILED" }), none)).toBe("failed");
    expect(taskSignal(task({ session_state: "FAILED" }), none)).toBe("failed");
    expect(taskSignal(task({ session_state: "FAILED", state: "COMPLETED" }), none)).toBe("other");
    expect(taskSignal(task({ queued_messages: 2, running_sessions: 1 }), none)).toBe("running");
    expect(taskSignal(task(), none)).toBe("waiting");
    expect(taskSignal(task({ session_id: "", session_state: "" }), none)).toBe("noSession");
  });
  it("sorts attention first and the most recent change first within a signal", () => {
    const rows = sortBySignal([
      { signal: "waiting" as const, task: task({ task_id: "a" }) },
      {
        signal: "failed" as const,
        task: task({ task_id: "b", updated_at: "2026-10-19T08:00:00Z" }),
      },
      {
        signal: "failed" as const,
        task: task({ task_id: "c", updated_at: "2026-10-19T09:00:00Z" }),
      },
      { signal: "stalled" as const, task: task({ task_id: "d" }) },
    ]);
    expect(rows.map((row) => row.task.task_id)).toEqual(["d", "c", "b", "a"]);
  });
});

describe("attention rules", () => {
  it("raises recorded failures and repeats without inventing stalls, once per task", () => {
    const items = buildAttention({
      ...base,
      figures: { w1: figures({ inProgress: 2, queued: 1 }) },
      tasks: [
        task({ task_id: "stall", queued_messages: 1, session_state: "FAILED" }),
        task({ task_id: "fail", session_state: "FAILED", updated_at: "2026-10-18T00:00:00Z" }),
        task({ task_id: "loop" }),
        task({ task_id: "fine" }),
      ],
      repeats: new Map([
        [
          "loop",
          { taskId: "loop", count: 12, start: "2026-10-19T09:00:00Z", end: "2026-10-19T09:50:00Z" },
        ],
      ]),
    });
    expect(
      items.map((item) => `${item.kind}:${item.scope}:${item.task?.task_id ?? item.workspaceId}`),
    ).toEqual(["repeat:task:loop", "failed:task:fail", "failed:task:stall"]);
    expect(countAttention(items)).toMatchObject({ all: 3, repeat: 1, stalled: 0, failed: 2 });
  });
  it("does not claim a stall while sessions wait on capacity or the wait is unknown", () => {
    const input = { ...base, figures: { w1: figures({ inProgress: 2 }) } };
    expect(buildAttention({ ...input, waiting: 1 })).toEqual([]);
    expect(buildAttention({ ...input, waiting: null })).toEqual([]);
  });
  it("keeps a repeat for a task outside the returned list and skips finished tasks", () => {
    const repeat = { taskId: "gone", workspaceId: "w1", count: 10, start: "a", end: "b" };
    const items = buildAttention({
      ...base,
      tasks: [task({ task_id: "done", state: "COMPLETED" })],
      repeats: new Map([
        ["gone", repeat],
        ["done", { ...repeat, taskId: "done" }],
      ]),
    });
    expect(items).toEqual([{ kind: "repeat", scope: "task", workspaceId: "w1", repeat }]);
  });
  it("flags coordinators that are on with none configured, only when the lists were read", () => {
    const on = { ...base, coordinators: { enabled: true, known: true, total: 0 } };
    expect(buildAttention(on)).toEqual([{ kind: "config", scope: "global", workspaceCount: 1 }]);
    expect(
      buildAttention({ ...on, coordinators: { enabled: true, known: false, total: 0 } }),
    ).toEqual([]);
    expect(
      buildAttention({ ...on, coordinators: { enabled: true, known: true, total: 1 } }),
    ).toEqual([]);
  });
  it("lists a workspace's waiting Needs-you items first and counts them", () => {
    const inbox = { answerable: 2, earlier: 1, recovery: 0, oldest: "2026-10-19T08:00:00Z" };
    const items = buildAttention({
      ...base,
      tasks: [task({ task_id: "f", state: "IN_PROGRESS", session_state: "FAILED" })],
      inbox: { w1: inbox },
    });
    expect(items[0]).toEqual({ kind: "question", scope: "workspace", workspaceId: "w1", inbox });
    expect(countAttention(items).question).toBe(1);
    expect(buildAttention({ ...base, inbox: {} })).toEqual([]);
  });
});
