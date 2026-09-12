import { describe, expect, it } from "vitest";
import {
  deriveWorkflowStepProgress,
  resolveWorkflowProgressEvidence,
} from "./use-workflow-step-progress";

const TASK_ID = "task-1";
const SESSION_ID = "session-primary";
const PROFILE_ID = "profile-primary";
const CONFIGURED_AGENT_LABEL = "Configured primary";

describe("deriveWorkflowStepProgress", () => {
  it("keeps the destination pending while a move request is unresolved", () => {
    expect(
      deriveWorkflowStepProgress({
        stepId: "review",
        currentStepId: "work",
        movingToStepId: "review",
        taskState: "IN_PROGRESS",
      }),
    ).toMatchObject({ status: "moving", isPending: true });
  });

  it("continues from settled move ownership through scheduling, startup, and running", () => {
    const settledMove = { stepId: "work", currentStepId: "work" };
    expect(deriveWorkflowStepProgress({ ...settledMove, taskState: "SCHEDULING" })).toMatchObject({
      status: "preparing",
      isPending: true,
    });
    expect(
      deriveWorkflowStepProgress({
        ...settledMove,
        taskState: "IN_PROGRESS",
        primarySessionState: "STARTING",
      }),
    ).toMatchObject({ status: "starting", isPending: true });
    expect(
      deriveWorkflowStepProgress({
        ...settledMove,
        taskState: "IN_PROGRESS",
        primarySessionState: "RUNNING",
      }),
    ).toMatchObject({ status: "running", isPending: false });
  });

  it.each([
    ["SCHEDULING", undefined, "preparing"],
    ["TODO", "STARTING", "starting"],
    ["IN_PROGRESS", "RUNNING", "running"],
    ["WAITING_FOR_INPUT", "WAITING_FOR_INPUT", "waiting"],
    ["IN_PROGRESS", "IDLE", "idle_agent"],
    ["COMPLETED", "RUNNING", "completed"],
    ["FAILED", "RUNNING", "failed"],
    ["CANCELLED", undefined, "cancelled"],
  ] as const)("maps %s/%s to %s", (taskState, sessionState, status) => {
    expect(
      deriveWorkflowStepProgress({
        stepId: "work",
        currentStepId: "work",
        taskState,
        primarySessionState: sessionState,
      }),
    ).toMatchObject({ status, isPending: ["preparing", "starting"].includes(status) });
  });

  it("does not infer startup from a CREATED session when the move has no auto-start", () => {
    expect(
      deriveWorkflowStepProgress({
        stepId: "work",
        currentStepId: "work",
        taskState: "TODO",
        primarySessionState: "CREATED",
      }),
    ).toMatchObject({ status: "not_started", isPending: false });
  });

  it("replaces startup progress with cancellation progress before terminal settlement", () => {
    expect(
      deriveWorkflowStepProgress({
        stepId: "work",
        currentStepId: "work",
        taskState: "SCHEDULING",
        primarySessionState: "STARTING",
        cancellationPending: true,
      }),
    ).toMatchObject({ status: "cancelling", isPending: false });

    expect(
      deriveWorkflowStepProgress({
        stepId: "work",
        currentStepId: "work",
        taskState: "IN_PROGRESS",
        primarySessionState: "CANCELLED",
        cancellationPending: false,
      }),
    ).toMatchObject({ status: "cancelled", isPending: false });
  });
});

describe("workflow progress step ownership", () => {
  it("does not apply current-step lifecycle to another step", () => {
    expect(
      deriveWorkflowStepProgress({
        stepId: "review",
        currentStepId: "work",
        taskState: "SCHEDULING",
        primarySessionState: "STARTING",
      }),
    ).toMatchObject({ status: "idle", isPending: false });
  });

  it("leaves unknown lifecycle evidence settled", () => {
    expect(
      deriveWorkflowStepProgress({
        stepId: "work",
        currentStepId: "work",
      }),
    ).toMatchObject({ status: "idle", isPending: false });
  });
});

describe("resolveWorkflowProgressEvidence", () => {
  it("uses the task primary session and ignores a historical session", () => {
    const result = resolveWorkflowProgressEvidence({
      task: {
        id: TASK_ID,
        primarySessionId: SESSION_ID,
        primarySessionState: "STARTING",
        primaryAgentProfileId: PROFILE_ID,
      },
      sessionsById: {
        "session-history": {
          id: "session-history",
          task_id: TASK_ID,
          state: "RUNNING",
          is_primary: false,
          agent_profile_id: "profile-history",
        },
        [SESSION_ID]: {
          id: SESSION_ID,
          task_id: TASK_ID,
          state: "STARTING",
          is_primary: true,
          agent_profile_id: PROFILE_ID,
          agent_profile_snapshot: { label: "Primary agent" },
        },
      },
      sessionsForTask: [],
      agentProfiles: [{ id: PROFILE_ID, label: CONFIGURED_AGENT_LABEL }],
    });

    expect(result).toEqual({
      sessionId: SESSION_ID,
      sessionState: "STARTING",
      cancellationPending: false,
      agentLabel: "Primary agent",
    });
  });

  it("falls back to the task projection while the session is unloaded", () => {
    const result = resolveWorkflowProgressEvidence({
      task: {
        id: TASK_ID,
        primarySessionId: SESSION_ID,
        primarySessionState: "STARTING",
        primaryAgentProfileId: PROFILE_ID,
      },
      sessionsById: {},
      sessionsForTask: [],
      agentProfiles: [{ id: PROFILE_ID, label: "Configured primary" }],
    });

    expect(result).toEqual({
      sessionId: SESSION_ID,
      sessionState: "STARTING",
      cancellationPending: false,
      agentLabel: CONFIGURED_AGENT_LABEL,
    });
  });

  it("uses a loaded primary session when the task projection has no session id", () => {
    const result = resolveWorkflowProgressEvidence({
      task: { id: TASK_ID, primaryAgentName: "Task agent" },
      sessionsById: {},
      sessionsForTask: [
        {
          id: SESSION_ID,
          task_id: TASK_ID,
          state: "WAITING_FOR_INPUT",
          is_primary: true,
          agent_profile_id: PROFILE_ID,
        },
      ],
      agentProfiles: [{ id: PROFILE_ID, label: CONFIGURED_AGENT_LABEL }],
    });

    expect(result).toEqual({
      sessionId: SESSION_ID,
      sessionState: "WAITING_FOR_INPUT",
      cancellationPending: false,
      agentLabel: "Configured primary",
    });
  });

  it("uses the global primary session cache when the task session list is unavailable", () => {
    const result = resolveWorkflowProgressEvidence({
      task: { id: TASK_ID },
      sessionsById: {
        [SESSION_ID]: {
          id: SESSION_ID,
          task_id: TASK_ID,
          state: "COMPLETED",
          is_primary: true,
          agent_profile_id: PROFILE_ID,
        },
      },
      sessionsForTask: [],
      agentProfiles: [{ id: PROFILE_ID, label: CONFIGURED_AGENT_LABEL }],
    });

    expect(result).toEqual({
      sessionId: SESSION_ID,
      sessionState: "COMPLETED",
      cancellationPending: false,
      agentLabel: CONFIGURED_AGENT_LABEL,
    });
  });
});

describe("workflow progress projection ownership", () => {
  it("does not select an arbitrary session when primary ownership is ambiguous", () => {
    const result = resolveWorkflowProgressEvidence({
      task: { id: TASK_ID },
      sessionsById: {
        [SESSION_ID]: {
          id: SESSION_ID,
          task_id: TASK_ID,
          state: "STARTING",
          is_primary: true,
        },
        "session-other-primary": {
          id: "session-other-primary",
          task_id: TASK_ID,
          state: "RUNNING",
          is_primary: true,
        },
      },
      sessionsForTask: [],
      agentProfiles: [],
    });

    expect(result).toEqual({
      sessionId: null,
      sessionState: null,
      cancellationPending: false,
      agentLabel: null,
    });
  });

  it("lets an overview task projection outrank a stale loaded session state", () => {
    const result = resolveWorkflowProgressEvidence({
      task: {
        id: TASK_ID,
        primarySessionId: SESSION_ID,
        primarySessionState: "STARTING",
      },
      sessionsById: {
        [SESSION_ID]: {
          id: SESSION_ID,
          task_id: TASK_ID,
          state: "RUNNING",
          is_primary: true,
        },
      },
      sessionsForTask: [],
      agentProfiles: [],
      preferTaskProjection: true,
    });

    expect(result.sessionState).toBe("STARTING");
  });
});

describe("workflow progress cancellation evidence", () => {
  it("carries authoritative cancellation evidence from the primary session", () => {
    const result = resolveWorkflowProgressEvidence({
      task: {
        id: TASK_ID,
        primarySessionId: SESSION_ID,
        primarySessionState: "STARTING",
      },
      sessionsById: {
        [SESSION_ID]: {
          id: SESSION_ID,
          task_id: TASK_ID,
          state: "STARTING",
          is_primary: true,
          cancellation_pending: true,
        },
      },
      sessionsForTask: [],
      agentProfiles: [],
    });

    expect(result).toMatchObject({
      sessionId: SESSION_ID,
      sessionState: "STARTING",
      cancellationPending: true,
    });
    expect(
      deriveWorkflowStepProgress({
        stepId: "work",
        currentStepId: "work",
        taskState: "SCHEDULING",
        primarySessionId: result.sessionId,
        primarySessionState: result.sessionState,
        cancellationPending: result.cancellationPending,
      }),
    ).toMatchObject({ status: "cancelling", isPending: false });
  });

  it("lets cancellation evidence override startup before terminal settlement", () => {
    expect(
      deriveWorkflowStepProgress({
        stepId: "work",
        currentStepId: "work",
        taskState: "SCHEDULING",
        primarySessionState: "STARTING",
        cancellationPending: true,
      }),
    ).toMatchObject({ status: "cancelling", isPending: false });

    expect(
      deriveWorkflowStepProgress({
        stepId: "work",
        currentStepId: "work",
        taskState: "IN_PROGRESS",
        primarySessionState: "CANCELLED",
      }),
    ).toMatchObject({ status: "cancelled", isPending: false });
  });

  it("ignores a previous terminal session when a task returns to TODO", () => {
    expect(
      deriveWorkflowStepProgress({
        stepId: "work",
        currentStepId: "work",
        taskState: "TODO",
        primarySessionState: "COMPLETED",
      }),
    ).toMatchObject({ status: "not_started", isPending: false });
  });
});
