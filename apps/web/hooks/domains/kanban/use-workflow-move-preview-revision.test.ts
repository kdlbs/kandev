import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { AppState } from "@/lib/state/store";
import {
  getWorkflowMovePreviewRevision,
  useWorkflowMovePreviewRevision,
} from "./use-workflow-move-preview-revision";

const TASK_ID = "task-1";
const WORKFLOW_ID = "workflow-1";
const SESSION_ID = "session-1";

type RevisionState = Pick<
  AppState,
  | "connection"
  | "workspaceContextGeneration"
  | "kanban"
  | "kanbanMulti"
  | "workflows"
  | "taskSessions"
  | "taskSessionsByTask"
  | "agentProfiles"
>;

let state: RevisionState;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (value: RevisionState) => string) => selector(state),
}));

function resetState() {
  state = {
    connection: { status: "connected", error: null, issueSeverity: "none" },
    workspaceContextGeneration: 1,
    kanban: {
      workflowId: WORKFLOW_ID,
      steps: [
        {
          id: "step-2",
          title: "Implement",
          color: "blue",
          position: 1,
          allow_manual_move: true,
          profile_session_start_policy: "reuse",
          events: { on_enter: [{ type: "auto_start_agent" }] },
        },
        {
          id: "step-1",
          title: "Analysis",
          color: "gray",
          position: 0,
          profile_session_end_policy: "park",
        },
      ],
      tasks: [
        {
          id: TASK_ID,
          workflowId: WORKFLOW_ID,
          workflowStepId: "step-1",
          title: "Task",
          position: 0,
          primarySessionId: SESSION_ID,
          primarySessionState: "WAITING_FOR_INPUT",
          updatedAt: "2026-09-15T00:00:00Z",
        },
      ],
      isLoading: false,
    },
    kanbanMulti: {
      snapshots: {},
      isLoading: false,
      orderRevisionByStepId: {},
      pendingReorderBandKeys: {},
      withheldReorderByBandKey: {},
    },
    workflows: {
      items: [{ id: WORKFLOW_ID, workspaceId: "workspace-1", name: "Workflow" }],
      activeId: WORKFLOW_ID,
    },
    taskSessions: { items: {} },
    taskSessionsByTask: {
      itemsByTaskId: {
        [TASK_ID]: [
          {
            id: SESSION_ID,
            task_id: TASK_ID,
            state: "WAITING_FOR_INPUT",
            started_at: "2026-09-15T00:00:00Z",
            updated_at: "2026-09-15T00:00:00Z",
            is_primary: true,
            agent_profile_id: "profile-1",
          },
        ],
      },
      loadingByTaskId: {},
      loadedByTaskId: { [TASK_ID]: true },
      errorByTaskId: {},
    },
    agentProfiles: { items: [], version: 1 },
  } as unknown as RevisionState;
}

describe("workflow move preview revisions", () => {
  it("changes for connection, session, task, step, and profile updates", () => {
    resetState();
    const first = getWorkflowMovePreviewRevision(state as AppState, TASK_ID, WORKFLOW_ID, "step-2");

    state.connection.status = "reconnecting";
    expect(
      getWorkflowMovePreviewRevision(state as AppState, TASK_ID, WORKFLOW_ID, "step-2"),
    ).not.toBe(first);

    state.connection.status = "connected";
    state.taskSessionsByTask.itemsByTaskId[TASK_ID]![0].state = "RUNNING";
    const afterSession = getWorkflowMovePreviewRevision(
      state as AppState,
      TASK_ID,
      WORKFLOW_ID,
      "step-2",
    );
    expect(afterSession).not.toBe(first);

    state.kanban.steps[0]!.profile_session_start_policy = "new";
    const afterStep = getWorkflowMovePreviewRevision(
      state as AppState,
      TASK_ID,
      WORKFLOW_ID,
      "step-2",
    );
    expect(afterStep).not.toBe(afterSession);

    state.kanban.steps[1]!.profile_session_end_policy = "complete";
    const afterSourceStep = getWorkflowMovePreviewRevision(
      state as AppState,
      TASK_ID,
      WORKFLOW_ID,
      "step-2",
    );
    expect(afterSourceStep).not.toBe(afterStep);

    state.agentProfiles.items = [
      {
        id: "profile-1",
        label: "Luna",
        agent_id: "agent-1",
        agent_name: "luna",
        cli_passthrough: false,
        model: "gpt-5.6-luna",
      },
    ];
    expect(
      getWorkflowMovePreviewRevision(state as AppState, TASK_ID, WORKFLOW_ID, "step-2"),
    ).not.toBe(afterSourceStep);
  });

  it("exposes the authoritative revision through the hook", () => {
    resetState();
    const { result, rerender } = renderHook(() =>
      useWorkflowMovePreviewRevision(TASK_ID, WORKFLOW_ID, "step-2"),
    );
    const first = result.current;
    state.connection.status = "disconnected";
    rerender();
    expect(result.current).not.toBe(first);
  });
});
