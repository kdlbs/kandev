import { renderHook } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import { registerAgentsHandlers } from "@/lib/ws/handlers/agents";
import type { AppState } from "@/lib/state/store";
import {
  getWorkflowMovePreviewRevision,
  useWorkflowMovePreviewRevision,
} from "./use-workflow-move-preview-revision";

const TASK_ID = "task-1";
const WORKFLOW_ID = "workflow-1";
const SESSION_ID = "session-1";
const INITIAL_PROFILE_ID = "profile-initial";
const BASE_TIMESTAMP = "2026-09-15T00:00:00Z";
const BOOKKEEPING_TIMESTAMP = "2026-09-15T00:01:00Z";

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
  | "settingsAgents"
  | "sessionModels"
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
          agent_profile_id: "profile-1",
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
          updatedAt: BASE_TIMESTAMP,
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
            started_at: BASE_TIMESTAMP,
            updated_at: BASE_TIMESTAMP,
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
    settingsAgents: { items: [] },
    sessionModels: {
      bySessionId: {
        [SESSION_ID]: {
          currentModelId: "gpt-5.6-luna",
          models: [],
          configOptions: [],
        },
      },
    },
  } as unknown as RevisionState;
}

function eventStore(): StoreApi<AppState> {
  const setState: StoreApi<AppState>["setState"] = (update) => {
    const next = typeof update === "function" ? update(state as AppState) : update;
    state = { ...state, ...next } as RevisionState;
  };
  return {
    getState: () => state as AppState,
    setState,
  } as unknown as StoreApi<AppState>;
}

function settingsProfile(id: string, overrides: Record<string, unknown> = {}) {
  return {
    id,
    name: id,
    agentId: "agent-1",
    agentDisplayName: "Luna",
    model: "mock-fast",
    mode: "default",
    configOptions: { reasoning_effort: "low" },
    allowIndexing: false,
    autoApprove: false,
    cliFlags: [],
    createdAt: BASE_TIMESTAMP,
    updatedAt: BASE_TIMESTAMP,
    ...overrides,
  } as never;
}

function profileEvent(id: string, overrides: Record<string, unknown> = {}) {
  return {
    id,
    agent_id: "agent-1",
    name: id,
    model: "mock-fast",
    mode: "plan",
    config_options: { reasoning_effort: "high" },
    enabled: true,
    created_at: "2026-09-15T00:00:00Z",
    updated_at: BOOKKEEPING_TIMESTAMP,
    ...overrides,
  };
}

function dispatchProfileUpdate(id: string, overrides: Record<string, unknown> = {}) {
  const handler = registerAgentsHandlers(eventStore())["agent.profile.updated"];
  if (!handler) throw new Error("agent.profile.updated handler is not registered");
  handler({
    id: `profile-update-${id}`,
    type: "notification",
    action: "agent.profile.updated",
    timestamp: BOOKKEEPING_TIMESTAMP,
    payload: { profile: profileEvent(id, overrides) },
  } as never);
}

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

  const beforeModel = getWorkflowMovePreviewRevision(
    state as AppState,
    TASK_ID,
    WORKFLOW_ID,
    "step-2",
  );
  state.sessionModels.bySessionId[SESSION_ID]!.currentModelId = "gpt-5.6-astra";
  expect(
    getWorkflowMovePreviewRevision(state as AppState, TASK_ID, WORKFLOW_ID, "step-2"),
  ).not.toBe(beforeModel);
});

it("ignores non-predictive task, session, and profile bookkeeping updates", () => {
  resetState();
  const first = getWorkflowMovePreviewRevision(state as AppState, TASK_ID, WORKFLOW_ID, "step-2");

  const task = state.kanban.tasks[0]!;
  task.description = "A newer description";
  task.updatedAt = BOOKKEEPING_TIMESTAMP;
  task.statusSummary = {
    updated_at: BOOKKEEPING_TIMESTAMP,
    status: "working",
  } as never;

  const session = state.taskSessionsByTask.itemsByTaskId[TASK_ID]![0]!;
  session.last_read_message_id = "message-2";
  session.command_count = 12;
  session.updated_at = BOOKKEEPING_TIMESTAMP;
  state.agentProfiles.version = 2;

  expect(getWorkflowMovePreviewRevision(state as AppState, TASK_ID, WORKFLOW_ID, "step-2")).toBe(
    first,
  );
});

it("normalizes equivalent projected map key order", () => {
  resetState();
  const session = state.taskSessionsByTask.itemsByTaskId[TASK_ID]![0]!;
  session.metadata = {
    runtime_config: {
      config_options: { reasoning_effort: "high", verbosity: "short" },
    },
  };
  const first = getWorkflowMovePreviewRevision(state as AppState, TASK_ID, WORKFLOW_ID, "step-2");

  session.metadata = {
    runtime_config: {
      config_options: { verbosity: "short", reasoning_effort: "high" },
    },
  };

  expect(getWorkflowMovePreviewRevision(state as AppState, TASK_ID, WORKFLOW_ID, "step-2")).toBe(
    first,
  );
});

it("tracks profile mode and options through profile events without global version churn", () => {
  resetState();
  state.agentProfiles.items = [
    {
      id: "profile-1",
      label: "Luna",
      agent_id: "agent-1",
      agent_name: "luna",
      model: "mock-fast",
      updatedAt: BASE_TIMESTAMP,
    },
    {
      id: "profile-2",
      label: "Other",
      agent_id: "agent-1",
      agent_name: "luna",
      model: "mock-fast",
      updatedAt: BASE_TIMESTAMP,
    },
  ] as never;
  state.settingsAgents.items = [
    {
      id: "agent-1",
      name: "luna",
      profiles: [settingsProfile("profile-1"), settingsProfile("profile-2")],
    },
  ] as never;

  const first = getWorkflowMovePreviewRevision(state as AppState, TASK_ID, WORKFLOW_ID, "step-2");
  dispatchProfileUpdate("profile-1", { config_options: { reasoning_effort: "low" } });
  const afterModeUpdate = getWorkflowMovePreviewRevision(
    state as AppState,
    TASK_ID,
    WORKFLOW_ID,
    "step-2",
  );
  expect(afterModeUpdate).not.toBe(first);

  dispatchProfileUpdate("profile-1", { updated_at: "2026-09-15T00:02:00Z" });
  const afterOptionsUpdate = getWorkflowMovePreviewRevision(
    state as AppState,
    TASK_ID,
    WORKFLOW_ID,
    "step-2",
  );
  expect(afterOptionsUpdate).not.toBe(afterModeUpdate);

  dispatchProfileUpdate("profile-2");
  expect(getWorkflowMovePreviewRevision(state as AppState, TASK_ID, WORKFLOW_ID, "step-2")).toBe(
    afterOptionsUpdate,
  );

  state.agentProfiles.version += 1;
  expect(getWorkflowMovePreviewRevision(state as AppState, TASK_ID, WORKFLOW_ID, "step-2")).toBe(
    afterOptionsUpdate,
  );
});

it("tracks a bound replacement profile before the destination session exists", () => {
  resetState();
  state.kanban.tasks[0]!.workflowAgentOverrides = {
    workflow_id: WORKFLOW_ID,
    steps: [
      {
        step_id: "step-2",
        source_profile_id: "profile-1",
        replacement_profile_id: "profile-replacement",
      },
    ],
  };
  state.agentProfiles.items = [
    {
      id: "profile-1",
      label: "Luna",
      agent_id: "agent-1",
      agent_name: "luna",
      cli_passthrough: false,
      model: "gpt-5.6-luna",
    },
    {
      id: "profile-replacement",
      label: "Terra",
      agent_id: "agent-2",
      agent_name: "terra",
      cli_passthrough: false,
      model: "gpt-5.6-terra",
    },
  ];

  const first = getWorkflowMovePreviewRevision(state as AppState, TASK_ID, WORKFLOW_ID, "step-2");
  const parsed = JSON.parse(first) as { profiles: Array<{ id: string; model?: string }> };
  expect(parsed.profiles.some(({ id }) => id === "profile-replacement")).toBe(true);

  state.agentProfiles.items[1]!.model = "gpt-5.6-terra-updated";
  expect(
    getWorkflowMovePreviewRevision(state as AppState, TASK_ID, WORKFLOW_ID, "step-2"),
  ).not.toBe(first);
});

it("tracks the initial-target profile after the original session is missing", () => {
  resetState();
  state.kanban.tasks[0]!.metadata = {
    workflow_initial_session: {
      session_id: "deleted-initial-session",
      agent_profile_id: INITIAL_PROFILE_ID,
    },
  };
  state.agentProfiles.items = [
    {
      id: "profile-1",
      label: "Luna",
      agent_id: "agent-1",
      agent_name: "luna",
      model: "mock-fast",
      updatedAt: BASE_TIMESTAMP,
    },
    {
      id: INITIAL_PROFILE_ID,
      label: "Initial Luna",
      agent_id: "agent-1",
      agent_name: "luna",
      model: "mock-fast",
      updatedAt: BASE_TIMESTAMP,
    },
  ] as never;
  state.settingsAgents.items = [
    {
      id: "agent-1",
      name: "luna",
      profiles: [settingsProfile("profile-1"), settingsProfile(INITIAL_PROFILE_ID)],
    },
  ] as never;

  const first = getWorkflowMovePreviewRevision(state as AppState, TASK_ID, WORKFLOW_ID, "step-2");
  const parsed = JSON.parse(first) as {
    original_sessions: Array<{ stores: Array<{ status: string }> }>;
    profiles: Array<{ id: string }>;
  };
  expect(parsed.original_sessions[0]?.stores.every(({ status }) => status === "missing")).toBe(
    true,
  );
  expect(parsed.profiles.some(({ id }) => id === INITIAL_PROFILE_ID)).toBe(true);

  dispatchProfileUpdate(INITIAL_PROFILE_ID, { mode: "plan" });
  expect(
    getWorkflowMovePreviewRevision(state as AppState, TASK_ID, WORKFLOW_ID, "step-2"),
  ).not.toBe(first);
});

it("deduplicates logical sessions across task and indexed stores", () => {
  resetState();
  const source = state.taskSessionsByTask.itemsByTaskId[TASK_ID]![0]!;
  source.metadata = { origin: "task_initial" };
  const candidateB = {
    ...source,
    id: "candidate-b" as never,
    metadata: undefined,
    is_primary: false,
    started_at: "2026-09-15T00:00:01Z",
    updated_at: "2026-09-15T00:00:10Z",
  };
  state.taskSessionsByTask.itemsByTaskId[TASK_ID]!.push(candidateB);
  state.taskSessions.items = {
    [source.id]: { ...source },
    [candidateB.id]: { ...candidateB },
  };

  const first = getWorkflowMovePreviewRevision(state as AppState, TASK_ID, WORKFLOW_ID, "step-2");
  const parsed = JSON.parse(first) as {
    original_sessions: Array<{
      stores: Array<{ status: string }>;
    }>;
    reusable_candidate_order: Array<{
      profiles: Array<{ sessions: Array<{ id: string }> }>;
    }>;
  };
  expect(parsed.original_sessions[0]?.stores.every(({ status }) => status === "selected")).toBe(
    true,
  );
  expect(
    parsed.reusable_candidate_order.every(({ profiles }) =>
      profiles.every(({ sessions }) => sessions.length === 1 && sessions[0]?.id === candidateB.id),
    ),
  ).toBe(true);

  const taskCandidate = state.taskSessionsByTask.itemsByTaskId[TASK_ID]![1]!;
  taskCandidate.updated_at = "2026-09-15T00:00:20Z";
  expect(getWorkflowMovePreviewRevision(state as AppState, TASK_ID, WORKFLOW_ID, "step-2")).toBe(
    first,
  );
  state.taskSessions.items[candidateB.id]!.updated_at = "2026-09-15T00:00:20Z";
  expect(getWorkflowMovePreviewRevision(state as AppState, TASK_ID, WORKFLOW_ID, "step-2")).toBe(
    first,
  );

  const candidateC = {
    ...candidateB,
    id: "candidate-c" as never,
    started_at: "2026-09-15T00:00:02Z",
    updated_at: "2026-09-15T00:00:15Z",
  };
  state.taskSessionsByTask.itemsByTaskId[TASK_ID]!.push(candidateC);
  state.taskSessions.items[candidateC.id] = { ...candidateC };
  const withTwoCandidates = getWorkflowMovePreviewRevision(
    state as AppState,
    TASK_ID,
    WORKFLOW_ID,
    "step-2",
  );
  taskCandidate.updated_at = "2026-09-15T00:00:09Z";
  expect(
    getWorkflowMovePreviewRevision(state as AppState, TASK_ID, WORKFLOW_ID, "step-2"),
  ).not.toBe(withTwoCandidates);
});

it("tracks candidate order changes without raw timestamp churn", () => {
  resetState();
  const source = state.taskSessionsByTask.itemsByTaskId[TASK_ID]![0]!;
  source.agent_profile_id = "profile-1" as never;
  source.is_primary = true;
  state.kanban.tasks[0]!.primarySessionId = source.id;
  state.taskSessionsByTask.itemsByTaskId[TASK_ID]!.push(
    {
      ...source,
      id: "candidate-a" as never,
      agent_profile_id: "profile-1" as never,
      is_primary: false,
      started_at: "2026-09-15T00:00:01Z",
      updated_at: "2026-09-15T00:00:10Z",
    },
    {
      ...source,
      id: "candidate-b" as never,
      agent_profile_id: "profile-1" as never,
      is_primary: false,
      started_at: "2026-09-15T00:00:02Z",
      updated_at: "2026-09-15T00:00:11Z",
    },
  );

  const first = getWorkflowMovePreviewRevision(state as AppState, TASK_ID, WORKFLOW_ID, "step-2");
  source.updated_at = BOOKKEEPING_TIMESTAMP;
  expect(getWorkflowMovePreviewRevision(state as AppState, TASK_ID, WORKFLOW_ID, "step-2")).toBe(
    first,
  );

  state.taskSessionsByTask.itemsByTaskId[TASK_ID]![2]!.updated_at = "2026-09-15T00:00:12Z";
  expect(getWorkflowMovePreviewRevision(state as AppState, TASK_ID, WORKFLOW_ID, "step-2")).toBe(
    first,
  );

  state.taskSessionsByTask.itemsByTaskId[TASK_ID]![2]!.updated_at = "2026-09-15T00:00:09Z";
  expect(
    getWorkflowMovePreviewRevision(state as AppState, TASK_ID, WORKFLOW_ID, "step-2"),
  ).not.toBe(first);
});

it("tracks terminal and completion-follow-up eligibility changes", () => {
  resetState();
  const source = state.taskSessionsByTask.itemsByTaskId[TASK_ID]![0]!;
  source.agent_profile_id = "profile-source" as never;
  state.taskSessionsByTask.itemsByTaskId[TASK_ID]!.push(
    {
      ...source,
      id: "eligible" as never,
      agent_profile_id: "profile-1" as never,
      is_primary: false,
      updated_at: "2026-09-15T00:00:10Z",
    },
    {
      ...source,
      id: "terminal" as never,
      agent_profile_id: "profile-1" as never,
      state: "COMPLETED",
      is_primary: false,
      updated_at: "2026-09-15T00:00:20Z",
    },
    {
      ...source,
      id: "follow-up" as never,
      agent_profile_id: "profile-1" as never,
      is_primary: false,
      metadata: { completion_follow_up: true },
      updated_at: "2026-09-15T00:00:30Z",
    },
  );

  const first = getWorkflowMovePreviewRevision(state as AppState, TASK_ID, WORKFLOW_ID, "step-2");
  const terminal = state.taskSessionsByTask.itemsByTaskId[TASK_ID]![2]!;
  terminal.state = "RUNNING";
  const afterTerminal = getWorkflowMovePreviewRevision(
    state as AppState,
    TASK_ID,
    WORKFLOW_ID,
    "step-2",
  );
  expect(afterTerminal).not.toBe(first);

  terminal.state = "COMPLETED";
  const followUp = state.taskSessionsByTask.itemsByTaskId[TASK_ID]![3]!;
  followUp.metadata = { completion_follow_up: false };
  expect(
    getWorkflowMovePreviewRevision(state as AppState, TASK_ID, WORKFLOW_ID, "step-2"),
  ).not.toBe(afterTerminal);
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
