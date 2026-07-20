/* eslint-disable max-lines-per-function, sonarjs/no-duplicate-string */
import { beforeEach, describe, expect, it, vi } from "vitest";
import { fetchFeatureFlags } from "@/lib/api/domains/features-api";
import { listTasksByWorkspace } from "@/lib/api/domains/kanban-api";
import { listWorkflowSteps } from "@/lib/api/domains/workflow-api";
import {
  fetchTaskSession,
  listTaskSessionMessages,
  listTaskSessions,
} from "@/lib/api/domains/session-api";
import { listTasks as listOfficeTasks } from "@/lib/api/domains/office-tasks-api";
import { getWorkspaceRouting } from "@/lib/api/domains/office-routing-api";
import { getTaskPlan, getPlanRevision, listPlanRevisions } from "@/lib/api/domains/plan-api";
import { getQueueStatus } from "@/lib/api/domains/queue-api";
import { featureFlagsQueryOptions } from "./features";
import { workflowStepsQueryOptions, workspaceTasksInfiniteQueryOptions } from "./kanban";
import {
  officeRoutingQueryOptions,
  officeTaskCommentsQueryOptions,
  officeTasksInfiniteQueryOptions,
  officeTaskSearchQueryOptions,
} from "./office";
import {
  planRevisionQueryOptions,
  queueStatusQueryOptions,
  sessionMessagesInfiniteQueryOptions,
  sessionMessagesLatestQueryOptions,
  taskPlanQueryOptions,
  taskPlanRevisionsQueryOptions,
  taskSessionQueryOptions,
  taskSessionsQueryOptions,
} from "./session";
import {
  fetchSessionCommitsSnapshot,
  sessionCommitsQueryOptions,
  sessionModelsQueryOptions,
  userShellsQueryOptions,
} from "./session-runtime";
import { qk } from "../keys";

vi.mock("@/lib/api/domains/features-api", () => ({
  fetchFeatureFlags: vi.fn(async () => ({ office: true })),
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  fetchTask: vi.fn(),
  fetchWorkflowSnapshot: vi.fn(),
  getSubtaskCount: vi.fn(),
  listTasksByWorkspace: vi.fn(async () => ({ tasks: [], total: 0 })),
  listWorkflows: vi.fn(),
}));

vi.mock("@/lib/api/domains/workflow-api", () => ({
  listWorkflowSteps: vi.fn(),
}));

vi.mock("@/lib/api/domains/session-api", () => ({
  fetchTaskSession: vi.fn(),
  listSessionTurns: vi.fn(),
  listTaskSessionMessages: vi.fn(async () => ({
    messages: [],
    total: 0,
    has_more: false,
    cursor: "",
  })),
  listTaskSessions: vi.fn(),
  searchSessionMessages: vi.fn(),
}));

vi.mock("@/lib/api/domains/plan-api", () => ({
  createTaskPlan: vi.fn(),
  deleteTaskPlan: vi.fn(),
  getPlanRevision: vi.fn(),
  getTaskPlan: vi.fn(),
  listPlanRevisions: vi.fn(),
  revertPlanRevision: vi.fn(),
  updateTaskPlan: vi.fn(),
}));

vi.mock("@/lib/api/domains/queue-api", () => ({
  clearQueue: vi.fn(),
  drainQueuedMessage: vi.fn(),
  getQueueStatus: vi.fn(),
  queueMessage: vi.fn(),
  removeQueuedEntry: vi.fn(),
  updateQueuedMessage: vi.fn(),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: vi.fn(),
}));

vi.mock("@/lib/api/domains/office-api", () => ({
  getCostsBreakdown: vi.fn(),
  getCostSummary: vi.fn(),
  getDashboard: vi.fn(),
  getInbox: vi.fn(),
  getMeta: vi.fn(),
  getProject: vi.fn(),
  getTask: vi.fn(),
  listAllRoutineRuns: vi.fn(),
  listActivity: vi.fn(),
  listActivityForTarget: vi.fn(),
  listAgentProfiles: vi.fn(),
  listBudgets: vi.fn(),
  listComments: vi.fn(async () => ({ comments: [] })),
  listProjects: vi.fn(),
  listRoutines: vi.fn(),
  listRoutineTriggers: vi.fn(),
  searchTasks: vi.fn(async () => ({ tasks: [] })),
}));

vi.mock("@/lib/api/domains/office-runs-api", () => ({
  getAgentSummary: vi.fn(),
  getRunAttempts: vi.fn(),
  getRunDetail: vi.fn(),
  listAgentRuns: vi.fn(),
  listRuns: vi.fn(),
}));

vi.mock("@/lib/api/domains/office-routing-api", () => ({
  getAgentRoute: vi.fn(),
  getProviderHealth: vi.fn(),
  getRoutingPreview: vi.fn(),
  getWorkspaceRouting: vi.fn(async () => ({ config: null, known_providers: [] })),
}));

vi.mock("@/lib/api/domains/office-skills-api", () => ({
  listSkills: vi.fn(),
}));

vi.mock("@/lib/api/domains/office-tasks-api", () => ({
  listTasks: vi.fn(async () => ({ tasks: [] })),
}));

function queryFnOf(options: { queryFn?: unknown }) {
  if (typeof options.queryFn !== "function") {
    throw new Error("queryFn was not a function");
  }
  return options.queryFn as (ctx: Record<string, unknown>) => Promise<unknown>;
}

describe("query option factories", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("maps feature flags to the feature API with the query abort signal", async () => {
    const signal = new AbortController().signal;
    const options = featureFlagsQueryOptions();

    await queryFnOf(options)({ signal });

    expect(fetchFeatureFlags).toHaveBeenCalledWith({ init: { signal } });
  });

  it("maps workspace task infinite pages to page params", async () => {
    const signal = new AbortController().signal;
    const options = workspaceTasksInfiniteQueryOptions("workspace-1", {
      pageSize: 2,
      query: "needle",
    });

    await queryFnOf(options)({ pageParam: 2, signal });

    expect(listTasksByWorkspace).toHaveBeenCalledWith(
      "workspace-1",
      { pageSize: 2, query: "needle", page: 2 },
      { init: { signal } },
    );
    expect(
      options.getNextPageParam?.(
        { tasks: [{ id: "task-2" }], total: 3 } as never,
        [
          { tasks: [{ id: "task-1" }], total: 3 },
          { tasks: [{ id: "task-2" }], total: 3 },
        ] as never,
        2,
        [1, 2],
      ),
    ).toBe(3);
  });

  it("maps workflow steps to a stable workflow-scoped key and sorted result", async () => {
    vi.mocked(listWorkflowSteps).mockResolvedValueOnce({
      steps: [
        { id: "step-2", name: "Second", workflow_id: "workflow-1", position: 2 },
        { id: "step-1", name: "First", workflow_id: "workflow-1", position: 1 },
      ],
    } as never);
    const signal = new AbortController().signal;
    const options = workflowStepsQueryOptions("workflow-1");

    const result = await queryFnOf(options)({ signal });

    expect(options.queryKey).toEqual(qk.workflows.steps("workflow-1"));
    expect(listWorkflowSteps).toHaveBeenCalledWith("workflow-1", { init: { signal } });
    expect(result).toEqual([
      expect.objectContaining({ id: "step-1" }),
      expect.objectContaining({ id: "step-2" }),
    ]);
  });

  it("maps session message cursors through page params", async () => {
    const signal = new AbortController().signal;
    const options = sessionMessagesInfiniteQueryOptions("session-1", { limit: 10 });

    await queryFnOf(options)({ pageParam: "message-1", signal });

    expect(listTaskSessionMessages).toHaveBeenCalledWith(
      "session-1",
      { limit: 10, before: "message-1" },
      { init: { signal } },
    );
    expect(
      options.getNextPageParam?.(
        { messages: [], total: 10, has_more: true, cursor: "message-0" } as never,
        [] as never,
        undefined,
        [],
      ),
    ).toBe("message-0");
  });

  it("maps latest session messages to the stable session message key in render order", async () => {
    vi.mocked(listTaskSessionMessages).mockResolvedValueOnce({
      messages: [
        { id: "new", created_at: "2026-06-23T00:00:02Z" },
        { id: "old", created_at: "2026-06-23T00:00:01Z" },
      ],
      total: 2,
      has_more: true,
      cursor: "old",
    } as never);
    const signal = new AbortController().signal;
    const options = sessionMessagesLatestQueryOptions("session-1", 2);

    const result = await queryFnOf(options)({ signal });

    expect(options.queryKey).toEqual(qk.session.messages("session-1"));
    expect(listTaskSessionMessages).toHaveBeenCalledWith(
      "session-1",
      { limit: 2, sort: "desc" },
      { init: { signal } },
    );
    expect(result).toMatchObject({
      hasMore: true,
      messages: [{ id: "old" }, { id: "new" }],
      oldestCursor: "old",
    });
  });

  it("unwraps task session by id responses to match hydrated query shape", async () => {
    vi.mocked(fetchTaskSession).mockResolvedValueOnce({
      session: { id: "session-1", task_id: "task-1", state: "RUNNING" },
    } as never);
    const signal = new AbortController().signal;
    const options = taskSessionQueryOptions("session-1");

    const result = await queryFnOf(options)({ signal });

    expect(options.queryKey).toEqual(qk.taskSession.byId("session-1"));
    expect(fetchTaskSession).toHaveBeenCalledWith("session-1", { init: { signal } });
    expect(result).toMatchObject({ id: "session-1", task_id: "task-1" });
  });

  it("maps task sessions, task plans, plan revisions, and queue status to stable keys", async () => {
    vi.mocked(listTaskSessions).mockResolvedValueOnce({ sessions: [], total: 0 } as never);
    vi.mocked(getTaskPlan).mockResolvedValueOnce({ id: "plan-1", task_id: "task-1" } as never);
    vi.mocked(listPlanRevisions).mockResolvedValueOnce([
      { id: "revision-1", task_id: "task-1", revision_number: 1 },
    ] as never);
    vi.mocked(getPlanRevision).mockResolvedValueOnce({
      id: "revision-1",
      task_id: "task-1",
      content: "content",
    } as never);
    vi.mocked(getQueueStatus).mockResolvedValueOnce({
      entries: [],
      count: 0,
      max: 10,
    });
    const signal = new AbortController().signal;

    expect(taskPlanQueryOptions("task-1").queryKey).toEqual(qk.taskPlan.detail("task-1"));
    expect(taskPlanRevisionsQueryOptions("task-1").queryKey).toEqual(
      qk.taskPlan.revisions("task-1"),
    );
    expect(planRevisionQueryOptions("task-1", "revision-1").queryKey).toEqual(
      qk.taskPlan.revision("task-1", "revision-1"),
    );
    expect(queueStatusQueryOptions("session-1").queryKey).toEqual(qk.session.queue("session-1"));

    await queryFnOf(taskSessionsQueryOptions("task-1"))({ signal });
    await queryFnOf(taskPlanQueryOptions("task-1"))({ signal });
    await queryFnOf(taskPlanRevisionsQueryOptions("task-1"))({ signal });
    await queryFnOf(planRevisionQueryOptions("task-1", "revision-1"))({ signal });
    await queryFnOf(queueStatusQueryOptions("session-1"))({ signal });

    expect(listTaskSessions).toHaveBeenCalledWith("task-1", { init: { signal } });
    expect(getTaskPlan).toHaveBeenCalledWith("task-1");
    expect(listPlanRevisions).toHaveBeenCalledWith("task-1");
    expect(getPlanRevision).toHaveBeenCalledWith("revision-1", "task-1");
    expect(getQueueStatus).toHaveBeenCalledWith("session-1");
  });

  it("maps session runtime snapshot queries to stable keys", async () => {
    const { getWebSocketClient } = await import("@/lib/ws/connection");
    const request = vi
      .fn()
      .mockResolvedValueOnce({ commits: [{ id: "commit-1" }], ready: true })
      .mockResolvedValueOnce({
        shells: [
          {
            id: "term-1",
            kind: "ordinary",
            seq: 1,
            display_name: "Terminal 1",
            state: "parked",
            pty_status: "running",
          },
        ],
      });
    vi.mocked(getWebSocketClient).mockReturnValue({ request } as never);

    expect(sessionCommitsQueryOptions("env-1", "session-1").queryKey).toEqual(
      qk.sessionRuntime.commits("env-1"),
    );
    expect(sessionModelsQueryOptions("session-1").queryKey).toEqual(
      qk.sessionRuntime.models("session-1"),
    );
    expect(userShellsQueryOptions("env-1", "task-1").queryKey).toEqual(
      qk.sessionRuntime.userShells("env-1", "task-1"),
    );

    await expect(fetchSessionCommitsSnapshot("session-1")).resolves.toEqual({
      commits: [{ id: "commit-1" }],
      ready: true,
    });
    await expect(queryFnOf(userShellsQueryOptions("env-1", "task-1"))({})).resolves.toEqual([
      expect.objectContaining({
        terminalId: "term-1",
        state: "parked",
        ptyStatus: "running",
      }),
    ]);
    expect(request).toHaveBeenNthCalledWith(1, "session.git.commits", {
      session_id: "session-1",
    });
    expect(request).toHaveBeenNthCalledWith(
      2,
      "user_shell.list",
      {
        task_environment_id: "env-1",
        include_parked: true,
        task_id: "task-1",
      },
      10000,
    );
  });

  it("maps office task cursors through page params", async () => {
    const signal = new AbortController().signal;
    const options = officeTasksInfiniteQueryOptions("workspace-1", {
      status: ["todo"],
      limit: 2,
    });

    await queryFnOf(options)({ pageParam: { cursor: "cursor-1", cursor_id: "task-1" }, signal });

    expect(listOfficeTasks).toHaveBeenCalledWith(
      "workspace-1",
      expect.objectContaining({
        status: ["todo"],
        sort: "updated_at",
        order: "desc",
        limit: 2,
        cursor: "cursor-1",
        cursor_id: "task-1",
      }),
      { init: { signal } },
    );
    expect(
      options.getNextPageParam?.(
        { tasks: [], next_cursor: "cursor-2", next_id: "task-2" } as never,
        [] as never,
        undefined,
        [],
      ),
    ).toEqual({ cursor: "cursor-2", cursor_id: "task-2" });
  });

  it("keeps multi-select office assignee and project filters out of unsupported request params", async () => {
    const signal = new AbortController().signal;
    const options = officeTasksInfiniteQueryOptions("workspace-1", {
      assignee: ["agent-1", "agent-2"],
      project: ["project-1", "project-2"],
      sort: null,
      order: null,
    });

    await queryFnOf(options)({ pageParam: undefined, signal });

    expect(listOfficeTasks).toHaveBeenCalledWith(
      "workspace-1",
      expect.not.objectContaining({
        assignee: expect.anything(),
        project: expect.anything(),
        sort: expect.anything(),
        order: expect.anything(),
      }),
      { init: { signal } },
    );
  });

  it("maps office routing queries through the routing API", async () => {
    const signal = new AbortController().signal;
    const options = officeRoutingQueryOptions("workspace-1");

    await queryFnOf(options)({ signal });

    expect(getWorkspaceRouting).toHaveBeenCalledWith("workspace-1", { init: { signal } });
  });

  it("maps office task comments through the office API", async () => {
    const signal = new AbortController().signal;
    const options = officeTaskCommentsQueryOptions("task-1");
    const { listComments } = await import("@/lib/api/domains/office-api");

    await queryFnOf(options)({ signal });

    expect(listComments).toHaveBeenCalledWith("task-1", { init: { signal } });
  });

  it("normalizes office task search queries before calling the API", async () => {
    const signal = new AbortController().signal;
    const options = officeTaskSearchQueryOptions("workspace-1", "  needle  ", 10);
    const { searchTasks } = await import("@/lib/api/domains/office-api");

    await queryFnOf(options)({ signal });

    expect(options.queryKey).toEqual(qk.office.taskSearch("workspace-1", "needle", 10));
    expect(searchTasks).toHaveBeenCalledWith("workspace-1", "needle", 10, {
      init: { signal },
    });
  });
});
