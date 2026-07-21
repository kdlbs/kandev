import { describe, expect, it } from "vitest";
import {
  buildAgentLabelsById,
  buildStepPositionById,
  buildStepTitleById,
  isSessionActive,
  pickActiveSessionId,
  resolveAgentLabelFor,
  resolveWorkflowStepTitle,
  sortSessions,
  sortSessionsByStepFlow,
} from "./session-sort";
import {
  agentProfileId as toAgentProfileId,
  sessionId as toSessionId,
  taskId as toTaskId,
  type TaskSession,
  type TaskSessionState,
} from "@/lib/types/http";

const EPOCH = "2025-01-01T00:00:00Z";
const D2 = "2025-01-02T00:00:00Z";
const D3 = "2025-01-03T00:00:00Z";
const D5 = "2025-01-05T00:00:00Z";
const D9 = "2025-01-09T00:00:00Z";

type SessionOverrides = Partial<Omit<TaskSession, "id" | "task_id" | "agent_profile_id">> & {
  id?: string;
  task_id?: string;
  agent_profile_id?: string;
};

function makeSession(overrides: SessionOverrides): TaskSession {
  const { id, task_id, agent_profile_id, ...rest } = overrides;
  return {
    id: toSessionId(id ?? "s1"),
    task_id: toTaskId(task_id ?? "t1"),
    ...(agent_profile_id !== undefined
      ? { agent_profile_id: toAgentProfileId(agent_profile_id) }
      : {}),
    environment_id: "e1",
    state: "CREATED" as TaskSessionState,
    started_at: EPOCH,
    updated_at: EPOCH,
    ...rest,
  } as TaskSession;
}

describe("sortSessions", () => {
  it("orders running sessions before completed ones", () => {
    const sessions = [
      makeSession({ id: "done", state: "COMPLETED", started_at: "2025-01-05T00:00:00Z" }),
      makeSession({ id: "run", state: "RUNNING", started_at: "2025-01-01T00:00:00Z" }),
    ];
    expect(sortSessions(sessions).map((s) => s.id)).toEqual(["run", "done"]);
  });

  it("breaks ties by most recent started_at", () => {
    const sessions = [
      makeSession({ id: "old", state: "RUNNING", started_at: "2025-01-01T00:00:00Z" }),
      makeSession({ id: "new", state: "RUNNING", started_at: "2025-01-05T00:00:00Z" }),
    ];
    expect(sortSessions(sessions).map((s) => s.id)).toEqual(["new", "old"]);
  });
});

describe("sortSessionsByStepFlow", () => {
  const stepPositionById = { spec: 0, work: 1, review: 2 };

  it("orders sessions by workflow-step position, then started_at", () => {
    const sessions = [
      makeSession({ id: "review", workflow_step_id: "review", started_at: EPOCH }),
      makeSession({ id: "spec-late", workflow_step_id: "spec", started_at: D9 }),
      makeSession({ id: "spec-early", workflow_step_id: "spec", started_at: D2 }),
      makeSession({ id: "work", workflow_step_id: "work", started_at: D3 }),
    ];
    expect(sortSessionsByStepFlow(sessions, stepPositionById).map((s) => s.id)).toEqual([
      "spec-early",
      "spec-late",
      "work",
      "review",
    ]);
  });

  it("sorts sessions with no known step last, by started_at", () => {
    const sessions = [
      makeSession({ id: "quick-late", started_at: D9 }),
      makeSession({ id: "spec", workflow_step_id: "spec", started_at: D5 }),
      makeSession({ id: "quick-early", started_at: EPOCH }),
      makeSession({
        id: "unknown-step",
        workflow_step_id: "missing",
        started_at: D2,
      }),
    ];
    expect(sortSessionsByStepFlow(sessions, stepPositionById).map((s) => s.id)).toEqual([
      "spec",
      "quick-early",
      "unknown-step",
      "quick-late",
    ]);
  });

  it("does not mutate the input array", () => {
    const sessions = [
      makeSession({ id: "work", workflow_step_id: "work" }),
      makeSession({ id: "spec", workflow_step_id: "spec" }),
    ];
    const snapshot = sessions.map((s) => s.id);
    sortSessionsByStepFlow(sessions, stepPositionById);
    expect(sessions.map((s) => s.id)).toEqual(snapshot);
  });
});

describe("workflow-step metadata helpers", () => {
  const state = {
    kanban: { steps: [{ id: "spec", position: 0, title: "Spec" }] },
    kanbanMulti: {
      snapshots: {
        wf1: {
          steps: [
            { id: "work", position: 1, title: "Work" },
            { id: "review", position: 2, title: "Review" },
          ],
        },
      },
    },
  };

  it("builds a merged { stepId -> position } map across boards", () => {
    expect(buildStepPositionById(state)).toEqual({ spec: 0, work: 1, review: 2 });
  });

  it("builds a merged { stepId -> title } map across boards", () => {
    expect(buildStepTitleById(state)).toEqual({ spec: "Spec", work: "Work", review: "Review" });
  });

  it("resolves a step title, or null when unknown", () => {
    expect(resolveWorkflowStepTitle(state, "review")).toBe("Review");
    expect(resolveWorkflowStepTitle(state, "spec")).toBe("Spec");
    expect(resolveWorkflowStepTitle(state, "missing")).toBeNull();
    expect(resolveWorkflowStepTitle(state, null)).toBeNull();
  });

  it("prefers the active board over a multi-board snapshot for the same step id, matching buildStepTitleById", () => {
    const conflicting = {
      kanban: { steps: [{ id: "shared", position: 0, title: "Active Title" }] },
      kanbanMulti: {
        snapshots: {
          wf1: { steps: [{ id: "shared", position: 5, title: "Snapshot Title" }] },
        },
      },
    };
    expect(resolveWorkflowStepTitle(conflicting, "shared")).toBe("Active Title");
    expect(buildStepTitleById(conflicting)).toEqual({ shared: "Active Title" });
  });
});

describe("resolveAgentLabelFor", () => {
  it("prefers the current store label when the profile still exists", () => {
    const session = makeSession({
      agent_profile_id: "p1",
      agent_profile_snapshot: { label: "Snapshot Agent" },
    });
    const labels = buildAgentLabelsById([{ id: "p1", label: "Store Agent" } as never]);
    expect(resolveAgentLabelFor(session, labels)).toBe("Store Agent");
  });

  it("falls back to snapshot label when the profile is no longer in the store", () => {
    const session = makeSession({
      agent_profile_id: "deleted",
      agent_profile_snapshot: { label: "Snapshot Agent" },
    });
    expect(resolveAgentLabelFor(session, {})).toBe("Snapshot Agent");
  });

  it("returns 'Unknown agent' when both are missing", () => {
    const session = makeSession({ agent_profile_id: "missing" });
    expect(resolveAgentLabelFor(session, {})).toBe("Unknown agent");
  });
});

describe("pickActiveSessionId", () => {
  it("returns null for an empty list", () => {
    expect(pickActiveSessionId([], "anything")).toBeNull();
  });

  it("honors a preferred session when it exists", () => {
    const sessions = [makeSession({ id: "a" }), makeSession({ id: "b", is_primary: true })];
    expect(pickActiveSessionId(sessions, "a")).toBe("a");
  });

  it("ignores the preferred session when not present and falls back to primary", () => {
    const sessions = [makeSession({ id: "a" }), makeSession({ id: "b", is_primary: true })];
    expect(pickActiveSessionId(sessions, "missing")).toBe("b");
  });

  it("falls back to the first session when no primary exists", () => {
    const sessions = [makeSession({ id: "a" }), makeSession({ id: "b" })];
    expect(pickActiveSessionId(sessions, null)).toBe("a");
  });
});

describe("isSessionActive", () => {
  it("is true while the agent is starting or running", () => {
    expect(isSessionActive("RUNNING")).toBe(true);
    expect(isSessionActive("STARTING")).toBe(true);
  });

  it("is false for terminal and idle states", () => {
    expect(isSessionActive("WAITING_FOR_INPUT")).toBe(false);
    expect(isSessionActive("COMPLETED")).toBe(false);
    expect(isSessionActive("FAILED")).toBe(false);
    expect(isSessionActive("CANCELLED")).toBe(false);
    expect(isSessionActive("CREATED")).toBe(false);
  });

  it("is false for null/undefined", () => {
    expect(isSessionActive(null)).toBe(false);
    expect(isSessionActive(undefined)).toBe(false);
  });
});
