import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { AgentProfileOption } from "@/lib/state/slices";
import { agentProfileId, workflowId, type Task, type WorkflowStepDTO } from "@/lib/types/http";
import {
  buildWorkflowChangePayload,
  normalizeChangeWorkflowOverrides,
  taskMatchesWorkflowChange,
  useChangeWorkflow,
} from "./use-change-workflow";

type HookStore = {
  workspaces: { items: Array<{ id: string; default_executor_id: string | null }> };
  kanban: { workflowId: string; steps: Array<{ id: string; title: string }> };
  kanbanMulti: { snapshots: Record<string, { steps: Array<{ id: string; title: string }> }> };
};

const changeWorkflowHookMocks = vi.hoisted(() => {
  const ids = {
    task: "task-1",
    workspace: "workspace-1",
    sourceWorkflow: "workflow-source",
    destinationWorkflow: "workflow-destination",
    sourceStep: "step-source",
    destinationStep: "step-pr",
    profileA: "profile-a",
    profileB: "profile-b",
  };
  return {
    ids,
    task: null as Task | null,
    profiles: [] as AgentProfileOption[],
    steps: [] as WorkflowStepDTO[],
    store: {} as HookStore,
  };
});

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: HookStore) => unknown) => selector(changeWorkflowHookMocks.store),
}));

vi.mock("@/components/task-create-dialog-options", () => ({
  useAgentProfileOptions: (profiles: AgentProfileOption[]) =>
    profiles.map((profile) => ({
      value: profile.id,
      label: profile.label,
      renderLabel: () => profile.label,
    })),
}));

vi.mock("@/hooks/domains/session/use-compatible-agent-profiles", () => ({
  useCompatibleAgentProfiles: (profiles: AgentProfileOption[]) => profiles,
}));

vi.mock("./use-change-workflow-data", () => ({
  useChangeWorkflowTask: () => ({
    task: changeWorkflowHookMocks.task,
    status: "success",
    error: null,
    refreshTask: async () => null,
  }),
  useChangeWorkflowCatalog: () => ({
    workflows: [
      {
        id: changeWorkflowHookMocks.ids.sourceWorkflow,
        workspace_id: changeWorkflowHookMocks.ids.workspace,
        name: "Source",
      },
      {
        id: changeWorkflowHookMocks.ids.destinationWorkflow,
        workspace_id: changeWorkflowHookMocks.ids.workspace,
        name: "Destination",
      },
    ],
    status: "success",
    error: null,
    retry: () => {},
  }),
  useChangeWorkflowProfiles: () => ({
    profiles: changeWorkflowHookMocks.profiles,
    executors: [],
    status: "success",
    error: null,
    retry: () => {},
  }),
  useChangeWorkflowDestination: () => ({
    selectedWorkflowId: changeWorkflowHookMocks.ids.destinationWorkflow,
    selectedStepId: changeWorkflowHookMocks.ids.destinationStep,
    overrides: {},
    snapshot: { steps: changeWorkflowHookMocks.steps },
    status: "success",
    error: null,
    changeWorkflow: () => {},
    chooseStep: () => {},
    setOverride: () => {},
    retry: () => {},
  }),
}));

vi.mock("./use-change-workflow-submit", () => ({
  useChangeWorkflowSubmit: () => ({
    isSubmitting: false,
    submitError: null,
    sourceProfileErrorId: undefined,
    uncertainResult: false,
    submit: async () => true,
    retryAfterRefresh: async () => {},
    clearSubmitError: () => {},
  }),
}));

const DESTINATION_WORKFLOW_ID = workflowId("workflow-destination");
const task = {
  id: "task-1",
  workspace_id: "workspace-1",
  workflow_id: "workflow-source",
  workflow_step_id: "step-source",
  updated_at: "2026-09-22T12:00:00Z",
  workflow_agent_overrides: {
    workflow_id: DESTINATION_WORKFLOW_ID,
    steps: [{ source_profile_id: "profile-a", replacement_profile_id: "profile-b" }],
  },
} as unknown as Task;

beforeEach(() => {
  changeWorkflowHookMocks.task = {
    id: changeWorkflowHookMocks.ids.task,
    workspace_id: changeWorkflowHookMocks.ids.workspace,
    workflow_id: changeWorkflowHookMocks.ids.sourceWorkflow,
    workflow_step_id: changeWorkflowHookMocks.ids.sourceStep,
    updated_at: "2026-09-22T12:00:00Z",
  } as unknown as Task;
  changeWorkflowHookMocks.profiles = [
    {
      id: changeWorkflowHookMocks.ids.profileA,
      label: "Agent A",
      agent_id: "agent-a",
      agent_name: "agent-a",
      cli_passthrough: false,
      enabled: true,
    },
    {
      id: changeWorkflowHookMocks.ids.profileB,
      label: "Agent B",
      agent_id: "agent-b",
      agent_name: "agent-b",
      cli_passthrough: false,
      enabled: true,
    },
  ];
  changeWorkflowHookMocks.steps = [];
  changeWorkflowHookMocks.store = {
    workspaces: {
      items: [{ id: changeWorkflowHookMocks.ids.workspace, default_executor_id: null }],
    },
    kanban: { workflowId: "", steps: [] },
    kanbanMulti: { snapshots: {} },
  };
});

describe("change-workflow request helpers", () => {
  it("builds a canonical payload from the current task version and draft mapping", () => {
    expect(
      buildWorkflowChangePayload(task, {
        " profile-z ": " profile-y ",
        "profile-a": "profile-a",
        "profile-empty": " ",
      }),
    ).toEqual({
      expected_workflow_id: "workflow-source",
      expected_step_id: "step-source",
      expected_updated_at: "2026-09-22T12:00:00Z",
      agent_overrides: { "profile-z": "profile-y" },
    });
  });

  it("normalizes map keys in stable order and drops self or empty mappings", () => {
    expect(
      normalizeChangeWorkflowOverrides({
        "profile-b": "replacement-b",
        "profile-a": "replacement-a",
        " profile-self ": " profile-self ",
        "profile-empty": "",
      }),
    ).toEqual({ "profile-a": "replacement-a", "profile-b": "replacement-b" });
  });

  it("recognizes a refreshed task that already has the requested destination and map", () => {
    const refreshedTask = {
      ...task,
      workflow_id: DESTINATION_WORKFLOW_ID,
      workflow_step_id: "step-target",
    } as Task;
    expect(
      taskMatchesWorkflowChange(refreshedTask, DESTINATION_WORKFLOW_ID, "step-target", {
        "profile-a": "profile-b",
      }),
    ).toBe(true);
    expect(
      taskMatchesWorkflowChange(refreshedTask, DESTINATION_WORKFLOW_ID, "step-target", {}),
    ).toBe(false);
  });
});

describe("useChangeWorkflow agent source derivation", () => {
  it("groups earlier-step recipients with their fixed source and ignores stale target profiles", () => {
    const implementStepId = "step-implement";
    changeWorkflowHookMocks.steps = [
      {
        id: implementStepId,
        workflow_id: workflowId(changeWorkflowHookMocks.ids.destinationWorkflow),
        name: "Implement",
        position: 0,
        agent_profile_id: agentProfileId(changeWorkflowHookMocks.ids.profileA),
        color: "",
        allow_manual_move: true,
        complete_task_on_enter: false,
        auto_advance_requires_signal: false,
        cancel_triggers_turn_complete: false,
      },
      {
        id: changeWorkflowHookMocks.ids.destinationStep,
        workflow_id: workflowId(changeWorkflowHookMocks.ids.destinationWorkflow),
        name: "PR",
        position: 1,
        agent_profile_id: agentProfileId("profile-pr-leftover"),
        session_target: { kind: "step", step_id: implementStepId },
        color: "",
        allow_manual_move: true,
        complete_task_on_enter: false,
        auto_advance_requires_signal: false,
        cancel_triggers_turn_complete: false,
      },
      {
        id: "step-initial",
        workflow_id: workflowId(changeWorkflowHookMocks.ids.destinationWorkflow),
        name: "Initial context",
        position: 2,
        agent_profile_id: agentProfileId("profile-initial-leftover"),
        session_target: { kind: "initial" },
        color: "",
        allow_manual_move: true,
        complete_task_on_enter: false,
        auto_advance_requires_signal: false,
        cancel_triggers_turn_complete: false,
      },
    ];

    const { result } = renderHook(() =>
      useChangeWorkflow({
        open: true,
        taskId: changeWorkflowHookMocks.ids.task,
        workspaceId: changeWorkflowHookMocks.ids.workspace,
        onOpenChange: () => {},
      }),
    );

    expect(result.current.rows).toEqual([
      expect.objectContaining({
        sourceProfileId: changeWorkflowHookMocks.ids.profileA,
        stepIds: [implementStepId, changeWorkflowHookMocks.ids.destinationStep],
        stepNames: ["Implement", "PR"],
      }),
    ]);
    expect(result.current.canSubmit).toBe(true);
  });
});
