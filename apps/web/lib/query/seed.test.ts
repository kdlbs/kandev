/* eslint-disable max-lines-per-function */
import { describe, expect, it } from "vitest";
import type { BootPayload } from "@/src/boot-payload";
import type { AgentProfile, Skill } from "@/lib/state/slices/office/types";
import {
  repositoryId as toRepositoryId,
  sessionId as toSessionId,
  taskId as toTaskId,
  workflowId as toWorkflowId,
  workspaceId as toWorkspaceId,
  type Message,
  type Repository,
  type RepositoryScript,
  type Task,
  type TaskSession,
  type Turn,
  type Workflow,
  type WorkflowSnapshot,
} from "@/lib/types/http";
import { makeQueryClient } from "./client";
import { qk } from "./keys";
import {
  seedQueryClientFromBootPayload,
  seedQueryClientFromInitialState,
  type QuerySeedInitialState,
} from "./seed";

const WORKSPACE_ID = "workspace-1";
const WORKFLOW_ID = "workflow-1";
const REVIEW_WORKFLOW_ID = "workflow-2";
const STEP_ID = "step-1";
const REVIEW_STEP_ID = "step-2";

describe("seedQueryClientFromBootPayload", () => {
  it("seeds generic boot state and feature flags", () => {
    const client = makeQueryClient();
    const payload = {
      initialState: {
        features: { office: true },
      },
    } satisfies BootPayload;

    seedQueryClientFromBootPayload(client, payload);

    expect(client.getQueryData(qk.boot.initialState())).toEqual(payload.initialState);
    expect(client.getQueryData(qk.features())).toEqual({ office: true });
  });

  it("seeds task detail route data into task, session, messages, and turns keys", () => {
    const client = makeQueryClient();
    const task = { id: "task-1", title: "Task", workspace_id: WORKSPACE_ID } as Task;
    const session = {
      id: "session-1",
      task_id: "task-1",
      repository_id: "repo-1",
      worktree_id: "worktree-1",
      worktree_path: "/tmp/kandev/worktrees/worktree-1",
      worktree_branch: "feature/session",
    } as TaskSession;
    const message = { id: "message-1", session_id: "session-1", content: "hello" } as Message;
    const turn = {
      id: "turn-1",
      session_id: "session-1",
      completed_at: null,
    } as unknown as Turn;
    const payload = {
      routeData: {
        taskDetail: {
          task,
          sessionId: "session-1",
          initialTerminals: [],
          initialState: {
            messages: {
              bySession: { "session-1": [message] },
              metaBySession: {
                "session-1": { hasMore: true, oldestCursor: "message-1", isLoading: false },
              },
            },
            taskSessions: { items: { "session-1": session } },
            taskSessionsByTask: {
              itemsByTaskId: { "task-1": [session] },
              loadingByTaskId: { "task-1": false },
              loadedByTaskId: { "task-1": true },
            },
            turns: {
              bySession: { "session-1": [turn] },
              activeBySession: { "session-1": "turn-1" },
            },
          },
        },
      },
    } satisfies BootPayload;

    seedQueryClientFromBootPayload(client, payload);

    expect(client.getQueryData(qk.tasks.detail("task-1"))).toEqual(task);
    expect(client.getQueryData(qk.taskSession.byTask("task-1"))).toEqual({
      sessions: [session],
    });
    expect(client.getQueryData(qk.taskSession.byId("session-1"))).toEqual(session);
    expect(client.getQueryData(qk.session.messages("session-1"))).toEqual({
      messages: [message],
      hasMore: true,
      oldestCursor: "message-1",
    });
    expect(client.getQueryData(qk.session.turns("session-1"))).toEqual({
      turns: [turn],
      activeTurnId: "turn-1",
    });
    expect(client.getQueryData(qk.sessionRuntime.worktrees("session-1"))).toEqual([
      {
        id: "worktree-1",
        sessionId: "session-1",
        repositoryId: "repo-1",
        path: "/tmp/kandev/worktrees/worktree-1",
        branch: "feature/session",
      },
    ]);
  });
});

describe("seedQueryClientFromInitialState", () => {
  it("seeds settings server-state into query keys", () => {
    const client = makeQueryClient();
    const executor = { id: "executor-1", name: "Docker" };
    const agent = {
      id: "agent-1",
      name: "codex",
      profiles: [{ id: "profile-1", agentDisplayName: "Codex", name: "Default" }],
    };
    const profile = {
      id: "profile-1",
      label: "Codex / Default",
      agent_id: "agent-1",
      agent_name: "codex",
      cli_passthrough: false,
    };
    const discoveryAgent = { name: "codex", display_name: "Codex" };
    const availableAgent = { name: "codex", display_name: "Codex", available: true };
    const tool = { name: "codex", installed: true };
    const editor = { id: "editor-1", name: "VS Code" };
    const prompt = { id: "prompt-1", name: "Review", content: "Check carefully." };
    const secret = { id: "secret-1", name: "TOKEN" };
    const spritesStatus = { configured: true, instance_count: 1 };
    const spritesInstance = { name: "sandbox-1" };
    const provider = { id: "provider-1", name: "Apprise" };

    const initialState = {
      executors: { items: [executor] },
      settingsAgents: { items: [agent] },
      agentProfiles: { items: [profile], version: 0 },
      agentDiscovery: {
        items: [discoveryAgent],
        loading: false,
        loaded: true,
      },
      availableAgents: {
        items: [availableAgent],
        tools: [tool],
        loading: false,
        loaded: true,
      },
      editors: { items: [editor], loading: false, loaded: true },
      prompts: { items: [prompt], loading: false, loaded: true },
      secrets: { items: [secret], loading: false, loaded: true },
      sprites: {
        status: spritesStatus,
        instances: [spritesInstance],
        loading: false,
        loaded: true,
      },
      notificationProviders: {
        items: [provider],
        events: ["task.created"],
        appriseAvailable: true,
        loading: false,
        loaded: true,
      },
    } as unknown as QuerySeedInitialState;

    seedQueryClientFromInitialState(client, initialState);

    expect(client.getQueryData(qk.settings.executors())).toEqual({ executors: [executor] });
    expect(client.getQueryData(qk.settings.agents())).toEqual({
      agents: [agent],
      total: 1,
    });
    expect(client.getQueryData(qk.settings.agentDiscovery())).toEqual({
      agents: [discoveryAgent],
      total: 1,
    });
    expect(client.getQueryData(qk.settings.availableAgents())).toEqual({
      agents: [availableAgent],
      tools: [tool],
      total: 1,
    });
    expect(client.getQueryData(qk.settings.editors())).toEqual({ editors: [editor] });
    expect(client.getQueryData(qk.settings.prompts())).toEqual({ prompts: [prompt] });
    expect(client.getQueryData(qk.settings.secrets())).toEqual([secret]);
    expect(client.getQueryData(qk.settings.spritesStatus())).toEqual(spritesStatus);
    expect(client.getQueryData(qk.settings.spritesInstances())).toEqual([spritesInstance]);
    expect(client.getQueryData(qk.settings.notificationProviders())).toEqual({
      providers: [provider],
      events: ["task.created"],
      apprise_available: true,
    });
  });

  it("can seed route-transition state from StateHydrator inputs", () => {
    const client = makeQueryClient();
    const message = {
      id: "message-2",
      session_id: "session-2",
      content: "from route",
    } as Message;

    seedQueryClientFromInitialState(
      client,
      {
        messages: {
          bySession: { "session-2": [message] },
          metaBySession: {
            "session-2": { hasMore: false, oldestCursor: "message-2", isLoading: false },
          },
        },
      },
      { sessionId: "session-2" },
    );

    expect(client.getQueryData(qk.session.messages("session-2"))).toEqual({
      messages: [message],
      hasMore: false,
      oldestCursor: "message-2",
    });
  });

  it("seeds workspace repositories into the workspace repositories query cache", () => {
    const client = makeQueryClient();
    const repository = {
      id: "repo-1",
      workspace_id: WORKSPACE_ID,
      name: "frontend",
      local_path: "/workspace/frontend",
    } as Repository;

    seedQueryClientFromInitialState(client, {
      repositories: {
        itemsByWorkspaceId: {
          [WORKSPACE_ID]: [repository],
        },
      },
    });

    expect(client.getQueryData(qk.workspaces.repositories(WORKSPACE_ID))).toEqual([repository]);
  });

  it("seeds workspace workflows into the workflow query cache", () => {
    const client = makeQueryClient();
    const workflow = {
      id: WORKFLOW_ID,
      workspace_id: WORKSPACE_ID,
      name: "Build",
      sort_order: 10,
      hidden: false,
    } as Workflow;

    seedQueryClientFromInitialState(client, {
      workflows: {
        items: [workflow],
        activeId: WORKFLOW_ID,
      },
    });

    expect(client.getQueryData(qk.workflows.all(WORKSPACE_ID, { includeHidden: true }))).toEqual([
      workflow,
    ]);
  });

  it("does not seed an empty office skills placeholder as fresh query data", () => {
    const client = makeQueryClient();

    seedQueryClientFromInitialState(client, {
      workspaces: { activeId: WORKSPACE_ID, items: [] },
      office: { skills: [] },
    } as unknown as QuerySeedInitialState);

    expect(client.getQueryData(qk.office.skills(WORKSPACE_ID))).toBeUndefined();
  });

  it("seeds non-empty office skills from route state", () => {
    const client = makeQueryClient();
    const skill = { id: "skill-1", slug: "kandev-protocol", name: "Kandev Protocol" } as Skill;

    seedQueryClientFromInitialState(client, {
      workspaces: { activeId: WORKSPACE_ID, items: [] },
      office: { skills: [skill] },
    } as unknown as QuerySeedInitialState);

    expect(client.getQueryData(qk.office.skills(WORKSPACE_ID))).toEqual({ skills: [skill] });
  });

  it("seeds office agents from the boot agentProfiles field", () => {
    const client = makeQueryClient();
    const profile = {
      id: "agent-profile-1",
      workspace_id: WORKSPACE_ID,
      name: "Planner",
    } as unknown as AgentProfile;

    seedQueryClientFromInitialState(client, {
      workspaces: { activeId: WORKSPACE_ID, items: [] },
      office: { agentProfiles: [profile] },
    } as unknown as QuerySeedInitialState);

    expect(client.getQueryData(qk.office.agents(WORKSPACE_ID))).toEqual({ agents: [profile] });
  });

  it("seeds workflow snapshots into the workflow snapshot query cache", () => {
    const client = makeQueryClient();
    const snapshot = {
      workflow: {
        id: toWorkflowId(WORKFLOW_ID),
        workspace_id: toWorkspaceId(WORKSPACE_ID),
        name: "Build",
        sort_order: 10,
        hidden: false,
        created_at: "",
        updated_at: "",
      },
      steps: [
        {
          id: STEP_ID,
          workflow_id: toWorkflowId(WORKFLOW_ID),
          name: "Todo",
          color: "bg-blue-500",
          position: 0,
          allow_manual_move: true,
        },
      ],
      tasks: [
        {
          id: toTaskId("task-1"),
          workspace_id: toWorkspaceId(WORKSPACE_ID),
          workflow_id: toWorkflowId(WORKFLOW_ID),
          workflow_step_id: STEP_ID,
          title: "Task",
          description: "",
          state: "TODO",
          priority: 0,
          position: 0,
          repositories: [],
          primary_session_id: toSessionId("session-1"),
          primary_session_state: "RUNNING",
          created_at: "",
          updated_at: "",
        },
      ],
    } as WorkflowSnapshot;

    seedQueryClientFromInitialState(client, {
      workflowSnapshots: {
        itemsByWorkflowId: {
          [WORKFLOW_ID]: snapshot,
        },
      },
    });

    expect(client.getQueryData<WorkflowSnapshot>(qk.workflows.snapshot(WORKFLOW_ID))).toEqual(
      snapshot,
    );
  });

  it("seeds multiple workflow snapshot query caches", () => {
    const client = makeQueryClient();
    const snapshot = {
      workflow: {
        id: toWorkflowId(REVIEW_WORKFLOW_ID),
        workspace_id: toWorkspaceId(WORKSPACE_ID),
        name: "Review",
        sort_order: 20,
        hidden: false,
        created_at: "",
        updated_at: "",
      },
      steps: [
        {
          id: REVIEW_STEP_ID,
          workflow_id: toWorkflowId(REVIEW_WORKFLOW_ID),
          name: "Review",
          color: "bg-green-500",
          position: 1,
          allow_manual_move: true,
        },
      ],
      tasks: [
        {
          id: toTaskId("task-2"),
          workspace_id: toWorkspaceId(WORKSPACE_ID),
          workflow_id: toWorkflowId(REVIEW_WORKFLOW_ID),
          workflow_step_id: REVIEW_STEP_ID,
          title: "Review task",
          description: "",
          state: "TODO",
          priority: 0,
          position: 0,
          repositories: [],
          created_at: "",
          updated_at: "",
        },
      ],
    } as WorkflowSnapshot;

    seedQueryClientFromInitialState(client, {
      workflowSnapshots: {
        itemsByWorkflowId: {
          [REVIEW_WORKFLOW_ID]: snapshot,
        },
      },
    });

    expect(
      client.getQueryData<WorkflowSnapshot>(qk.workflows.snapshot(REVIEW_WORKFLOW_ID)),
    ).toEqual(snapshot);
  });

  it("does not treat workflow snapshot seeds as Zustand kanban state", () => {
    const client = makeQueryClient();
    const workflow = {
      id: WORKFLOW_ID,
      workspace_id: WORKSPACE_ID,
      name: "Build",
      sort_order: 10,
      hidden: false,
    } as Workflow;

    seedQueryClientFromInitialState(client, {
      workspaces: {
        activeId: WORKSPACE_ID,
        items: [],
      },
      workflows: {
        items: [workflow],
        activeId: WORKFLOW_ID,
      },
      workflowSnapshots: {
        itemsByWorkflowId: {},
      },
      // @ts-expect-error Legacy store mirrors are intentionally no longer part of the seed API.
      kanban: {
        workflowId: WORKFLOW_ID,
        isLoading: false,
        steps: [{ id: STEP_ID, title: "Todo", color: "bg-blue-500", position: 0 }],
        tasks: [
          {
            id: "task-1",
            workflowStepId: STEP_ID,
            title: "Task",
            position: 0,
            primarySessionId: "session-1",
            primarySessionState: "RUNNING",
          },
        ],
      },
    });

    expect(client.getQueryData<WorkflowSnapshot>(qk.workflows.snapshot(WORKFLOW_ID))).toEqual(
      undefined,
    );
  });

  it("seeds repository scripts into the repository scripts query cache", () => {
    const client = makeQueryClient();
    const script = {
      id: "script-1",
      repository_id: toRepositoryId("repo-1"),
      name: "Setup",
      command: "pnpm install",
      position: 0,
      created_at: "2026-06-24T00:00:00Z",
      updated_at: "2026-06-24T00:00:00Z",
    } satisfies RepositoryScript;

    seedQueryClientFromInitialState(client, {
      repositoryScripts: {
        itemsByRepositoryId: {
          "repo-1": [script],
        },
      },
    });

    expect(client.getQueryData(qk.workspaces.repositoryScripts("repo-1"))).toEqual([script]);
  });
});
