/* eslint-disable max-lines, max-lines-per-function, sonarjs/no-duplicate-string */
import { QueryClient } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import type { BackendMessageMap, BackendMessageType } from "@/lib/types/backend";
import type { BackendMessage } from "@/lib/types/backend-message";
import type { TaskPR } from "@/lib/types/github";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  workflowId as toWorkflowId,
  workspaceId as toWorkspaceId,
  type Task,
  type WorkflowSnapshot,
} from "@/lib/types/http";
import type { WebSocketClient } from "@/lib/ws/client";
import { makeQueryClient } from "../client";
import { qk } from "../keys";
import {
  BRIDGE_SKIPPED_ACTIONS,
  clearBridgeAuditRows,
  getBridgeAuditRows,
  isBridgeSkippedAction,
  registerQueryBridge,
} from "./index";

type BridgeWindow = Window & {
  __KANDEV_E2E_EXPOSE_STORE__?: boolean;
  __kandev_bridge_audit__?: () => unknown[];
  __kandev_bridge_audit_clear__?: () => void;
};

type AnyBackendMessage = BackendMessage<string, Record<string, unknown>>;
type Handler = (message: AnyBackendMessage) => void;

class FakeWebSocketClient {
  private envelopeHandlers = new Set<Handler>();
  private handlers = new Map<string, Set<Handler>>();

  on<T extends BackendMessageType>(type: T, handler: (message: BackendMessageMap[T]) => void) {
    const bucket = this.handlers.get(type) ?? new Set<Handler>();
    bucket.add(handler as Handler);
    this.handlers.set(type, bucket);
    return () => {
      bucket.delete(handler as Handler);
    };
  }

  onEnvelope(handler: (message: BackendMessageMap[BackendMessageType]) => void) {
    this.envelopeHandlers.add(handler as Handler);
    return () => {
      this.envelopeHandlers.delete(handler as Handler);
    };
  }

  emit(message: AnyBackendMessage) {
    this.envelopeHandlers.forEach((handler) => handler(message));
    this.handlers.get(message.action)?.forEach((handler) => handler(message));
  }
}

function taskUpdated(
  overrides: Partial<BackendMessageMap["task.updated"]["payload"]> = {},
): BackendMessageMap["task.updated"] {
  return {
    type: "notification",
    action: "task.updated",
    payload: {
      task_id: "task-1",
      workflow_id: "workflow-1",
      workflow_step_id: "step-2",
      title: "Updated task",
      is_ephemeral: false,
      ...overrides,
    },
  };
}

function workflowSnapshotTask(overrides: Partial<Task> = {}): Task {
  return {
    id: toTaskId("task-1"),
    workspace_id: toWorkspaceId("workspace-1"),
    workflow_id: toWorkflowId("workflow-1"),
    workflow_step_id: "step-1",
    position: 0,
    title: "Task",
    description: "",
    state: "TODO",
    priority: 0,
    primary_session_id: toSessionId("session-1"),
    primary_session_state: "WAITING_FOR_INPUT",
    repositories: [],
    created_at: "2026-06-24T00:00:00Z",
    updated_at: "2026-06-24T00:00:00Z",
    ...overrides,
  } as Task;
}

function workflowSnapshot(tasks: Task[]): WorkflowSnapshot {
  return {
    workflow: {
      id: toWorkflowId("workflow-1"),
      workspace_id: toWorkspaceId("workspace-1"),
      name: "Workflow",
      sort_order: 0,
      hidden: false,
      created_at: "2026-06-24T00:00:00Z",
      updated_at: "2026-06-24T00:00:00Z",
    },
    steps: [
      {
        id: "step-1",
        workflow_id: toWorkflowId("workflow-1"),
        name: "Todo",
        position: 0,
        color: "bg-blue-500",
        allow_manual_move: true,
      },
    ],
    tasks,
  };
}

function taskPr(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "task-pr-1",
    task_id: "task-1",
    repository_id: "repo-1",
    owner: "kdlbs",
    repo: "kandev",
    pr_number: 1512,
    pr_url: "https://github.com/kdlbs/kandev/pull/1512",
    pr_title: "Old title",
    head_branch: "feature/tanstack-migration-801",
    base_branch: "main",
    author_login: "octocat",
    state: "open",
    review_state: "pending",
    checks_state: "pending",
    mergeable_state: "unknown",
    review_count: 0,
    pending_review_count: 0,
    required_reviews: null,
    comment_count: 0,
    unresolved_review_threads: 0,
    checks_total: 0,
    checks_passing: 0,
    additions: 0,
    deletions: 0,
    created_at: "2026-06-24T00:00:00Z",
    merged_at: null,
    closed_at: null,
    last_synced_at: "2026-06-24T00:00:00Z",
    updated_at: "2026-06-24T00:00:00Z",
    ...overrides,
  };
}

function registerBridge(ws: FakeWebSocketClient, client: QueryClient) {
  return registerQueryBridge(ws as unknown as WebSocketClient, client);
}

describe("query bridge audit", () => {
  beforeEach(() => {
    (window as BridgeWindow).__KANDEV_E2E_EXPOSE_STORE__ = true;
    clearBridgeAuditRows();
    delete (window as BridgeWindow).__kandev_bridge_audit__;
    delete (window as BridgeWindow).__kandev_bridge_audit_clear__;
  });

  afterEach(() => {
    clearBridgeAuditRows();
    delete (window as BridgeWindow).__KANDEV_E2E_EXPOSE_STORE__;
    delete (window as BridgeWindow).__kandev_bridge_audit__;
    delete (window as BridgeWindow).__kandev_bridge_audit_clear__;
  });

  it("patches registered query keys and records handled audit rows", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.tasks.detail("task-1"), {
      id: "task-1",
      title: "Old title",
      workflow_step_id: "step-1",
    });

    const cleanup = registerBridge(ws, queryClient);
    ws.emit(taskUpdated());

    expect(queryClient.getQueryData(qk.tasks.detail("task-1"))).toMatchObject({
      id: "task-1",
      title: "Updated task",
      workflow_step_id: "step-2",
    });
    expect(getBridgeAuditRows()).toEqual([
      expect.objectContaining({
        action: "task.updated",
        cacheChanged: true,
        mutationCount: expect.any(Number),
        status: "handled",
        taskId: "task-1",
      }),
    ]);
    expect((window as BridgeWindow).__kandev_bridge_audit__?.()).toHaveLength(1);

    cleanup();
  });

  it("records allowlisted envelopes without registering cache handlers", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      id: "response-1",
      type: "response",
      action: "session.subscribe",
      payload: { session_id: "session-1" },
    });

    expect(getBridgeAuditRows()).toEqual([
      expect.objectContaining({
        action: "session.subscribe",
        cacheChanged: false,
        mutationCount: 0,
        reason: BRIDGE_SKIPPED_ACTIONS["session.subscribe"],
        sessionId: "session-1",
        status: "allowlisted",
      }),
    ]);
    expect(isBridgeSkippedAction("session.subscribe")).toBe(true);

    cleanup();
  });

  it("handles repository events by invalidating workspace repository caches", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.workspaces.repositories("workspace-1"), [{ id: "repo-1" }]);
    queryClient.setQueryData(qk.workspaces.repositories("workspace-1", { includeScripts: true }), [
      { id: "repo-1", scripts: [] },
    ]);
    queryClient.setQueryData(qk.workspaces.repositoryScripts("repo-1"), []);

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "repository.updated",
      payload: {
        id: "repo-1",
        workspace_id: "workspace-1",
        name: "Updated repository",
      },
    });

    expect(
      queryClient.getQueryState(qk.workspaces.repositories("workspace-1"))?.isInvalidated,
    ).toBe(true);
    expect(
      queryClient.getQueryState(qk.workspaces.repositories("workspace-1", { includeScripts: true }))
        ?.isInvalidated,
    ).toBe(true);
    expect(
      queryClient.getQueryState(qk.workspaces.repositoryScripts("repo-1"))?.isInvalidated,
    ).toBe(true);
    expect(getBridgeAuditRows()).toContainEqual(
      expect.objectContaining({
        action: "repository.updated",
        status: "handled",
      }),
    );

    cleanup();
  });

  it("patches cached workflow lists when a workflow is created", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.workflows.all("workspace-1"), [
      {
        id: "workflow-1",
        workspace_id: "workspace-1",
        name: "Existing",
        hidden: false,
      },
    ]);
    queryClient.setQueryData(qk.workflows.all("workspace-2"), [
      {
        id: "workflow-2",
        workspace_id: "workspace-2",
        name: "Other workspace",
        hidden: false,
      },
    ]);

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "workflow.created",
      payload: {
        id: "workflow-new",
        workspace_id: "workspace-1",
        name: "New workflow",
        hidden: false,
      },
    });

    expect(queryClient.getQueryData(qk.workflows.all("workspace-1"))).toEqual([
      expect.objectContaining({ id: "workflow-1" }),
      expect.objectContaining({ id: "workflow-new", name: "New workflow" }),
    ]);
    expect(queryClient.getQueryData(qk.workflows.all("workspace-2"))).toEqual([
      expect.objectContaining({ id: "workflow-2" }),
    ]);
    expect(queryClient.getQueryState(qk.workflows.all("workspace-1"))?.isInvalidated).toBe(true);

    cleanup();
  });

  it("handles repository script events by invalidating script and repository-list caches", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.workspaces.repositories("workspace-1"), [
      { id: "repo-1", scripts: [] },
    ]);
    queryClient.setQueryData(qk.workspaces.repositoryScripts("repo-1"), [
      { id: "script-1", repository_id: "repo-1" },
    ]);

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "repository.script.updated",
      payload: {
        id: "script-1",
        repository_id: "repo-1",
        name: "Setup",
      },
    });

    expect(
      queryClient.getQueryState(qk.workspaces.repositories("workspace-1"))?.isInvalidated,
    ).toBe(true);
    expect(
      queryClient.getQueryState(qk.workspaces.repositoryScripts("repo-1"))?.isInvalidated,
    ).toBe(true);
    expect(getBridgeAuditRows()).toContainEqual(
      expect.objectContaining({
        action: "repository.script.updated",
        status: "handled",
      }),
    );

    cleanup();
  });

  it("patches office task query pages and records handled office audit rows", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.office.tasks("workspace-1", { limit: 200 }), {
      pages: [
        {
          tasks: [
            {
              id: "office-task-1",
              title: "Old office title",
              status: "todo",
            },
          ],
        },
      ],
      pageParams: [undefined],
    });

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "office.task.updated",
      payload: {
        workspace_id: "workspace-1",
        task_id: "office-task-1",
        title: "Updated office title",
      },
    });

    expect(queryClient.getQueryData(qk.office.tasks("workspace-1", { limit: 200 }))).toMatchObject({
      pages: [{ tasks: [{ id: "office-task-1", title: "Updated office title" }] }],
    });
    expect(getBridgeAuditRows()).toContainEqual(
      expect.objectContaining({
        action: "office.task.updated",
        status: "handled",
        taskId: "office-task-1",
      }),
    );

    cleanup();
  });

  it("patches task PR rows and invalidates workspace PR aggregates without cross-workspace writes", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    const oldPr = taskPr();
    const updatedPr = taskPr({ pr_title: "Updated title", checks_state: "success" });
    const workspacePrsKey = qk.integrations.github.prs("workspace-1");
    const otherWorkspacePrsKey = qk.integrations.github.prs("workspace-2");
    const otherWorkspacePr = taskPr({ id: "task-pr-2", task_id: "task-2", pr_number: 99 });
    queryClient.setQueryData(qk.integrations.github.taskPr("task-1"), [oldPr]);
    queryClient.setQueryData(workspacePrsKey, { task_prs: { "task-1": [oldPr] } });
    queryClient.setQueryData(otherWorkspacePrsKey, { task_prs: { "task-2": [otherWorkspacePr] } });

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "github.task_pr.updated",
      payload: updatedPr,
    });

    expect(queryClient.getQueryData(qk.integrations.github.taskPr("task-1"))).toEqual([
      expect.objectContaining({
        checks_state: "success",
        pr_title: "Updated title",
      }),
    ]);
    expect(queryClient.getQueryData(workspacePrsKey)).toEqual({
      task_prs: {
        "task-1": [oldPr],
      },
    });
    expect(queryClient.getQueryData(otherWorkspacePrsKey)).toEqual({
      task_prs: {
        "task-2": [otherWorkspacePr],
      },
    });
    expect(
      queryClient.getQueryData<{ task_prs: Record<string, TaskPR[]> }>(otherWorkspacePrsKey)
        ?.task_prs["task-1"],
    ).toBeUndefined();
    expect(queryClient.getQueryState(workspacePrsKey)?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(otherWorkspacePrsKey)?.isInvalidated).toBe(true);

    cleanup();
  });

  it("upserts provider health rows from office bridge events", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.office.providerHealth("workspace-1"), { health: [] });

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "office.provider.health_changed",
      payload: {
        workspace_id: "workspace-1",
        provider_id: "claude-acp",
        scope: "provider",
        scope_value: "",
        state: "degraded",
        backoff_step: 1,
      },
    });

    expect(queryClient.getQueryData(qk.office.providerHealth("workspace-1"))).toEqual({
      health: [
        expect.objectContaining({
          provider_id: "claude-acp",
          state: "degraded",
        }),
      ],
    });

    cleanup();
  });

  it("does not materialize partial provider health lists from office bridge events", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "office.provider.health_changed",
      payload: {
        workspace_id: "workspace-1",
        provider_id: "claude-acp",
        scope: "provider",
        scope_value: "",
        state: "degraded",
        backoff_step: 1,
      },
    });

    expect(queryClient.getQueryData(qk.office.providerHealth("workspace-1"))).toBeUndefined();
    expect(queryClient.getQueryState(qk.office.providerHealth("workspace-1"))).toBeUndefined();

    cleanup();
  });

  it("appends route attempts from office bridge events", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.office.runAttempts("run-1"), { attempts: [] });

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "office.route_attempt.appended",
      payload: {
        run_id: "run-1",
        attempt: {
          seq: 1,
          provider_id: "codex",
          tier: "balanced",
          outcome: "launched",
          started_at: "2026-06-23T00:00:00Z",
        },
      },
    });

    expect(queryClient.getQueryData(qk.office.runAttempts("run-1"))).toEqual({
      attempts: [
        expect.objectContaining({
          provider_id: "codex",
          seq: 1,
        }),
      ],
    });

    cleanup();
  });

  it("does not seed full run-attempt caches from live route attempt events", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "office.route_attempt.appended",
      payload: {
        run_id: "run-1",
        attempt: {
          seq: 1,
          provider_id: "codex",
          tier: "balanced",
          outcome: "launched",
          started_at: "2026-06-23T00:00:00Z",
        },
      },
    });

    expect(queryClient.getQueryData(qk.office.runAttempts("run-1"))).toBeUndefined();

    cleanup();
  });

  it("invalidates office task comments when comment events arrive", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.office.taskComments("office-task-1"), { comments: [] });

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "office.comment.created",
      payload: {
        task_id: "office-task-1",
      },
    });

    expect(queryClient.getQueryState(qk.office.taskComments("office-task-1"))?.isInvalidated).toBe(
      true,
    );

    cleanup();
  });

  it("invalidates office task comments when run lifecycle events arrive", () => {
    const events: Array<"office.run.queued" | "office.run.processed"> = [
      "office.run.queued",
      "office.run.processed",
    ];

    for (const action of events) {
      const ws = new FakeWebSocketClient();
      const queryClient = makeQueryClient();
      queryClient.setQueryData(qk.office.taskComments("office-task-1"), { comments: [] });

      const cleanup = registerBridge(ws, queryClient);
      ws.emit({
        type: "notification",
        action,
        payload: {
          workspace_id: "workspace-1",
          task_id: "office-task-1",
          run_id: "run-1",
        },
      });

      expect(
        queryClient.getQueryState(qk.office.taskComments("office-task-1"))?.isInvalidated,
      ).toBe(true);

      cleanup();
    }
  });

  it("invalidates linked office task surfaces when task events arrive", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.office.projects("workspace-1"), { projects: [] });
    queryClient.setQueryData(qk.office.project("project-1"), { project: { id: "project-1" } });
    queryClient.setQueryData(qk.office.agentSummary("agent-1", 14), { summary: {} });

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "office.task.status_changed",
      payload: {
        workspace_id: "workspace-1",
        task_id: "office-task-1",
        project_id: "project-1",
        assignee_agent_profile_id: "agent-1",
        new_status: "done",
      },
    });

    expect(queryClient.getQueryState(qk.office.projects("workspace-1"))?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(qk.office.project("project-1"))?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(qk.office.agentSummary("agent-1", 14))?.isInvalidated).toBe(
      true,
    );

    cleanup();
  });

  it("invalidates office agent and task run surfaces when run lifecycle events arrive", () => {
    const events: Array<"office.run.queued" | "office.run.processed"> = [
      "office.run.queued",
      "office.run.processed",
    ];

    for (const action of events) {
      const ws = new FakeWebSocketClient();
      const queryClient = makeQueryClient();
      queryClient.setQueryData(qk.office.dashboard("workspace-1"), {});
      queryClient.setQueryData(qk.office.agentSummary("agent-1", 14), { summary: {} });
      queryClient.setQueryData(qk.office.agentRuns("agent-1", { limit: 25 }), { runs: [] });
      queryClient.setQueryData(qk.office.runDetail("agent-1", "run-1"), { run: { id: "run-1" } });
      queryClient.setQueryData(qk.office.taskActivity("workspace-1", "office-task-1"), {
        activity: [],
      });

      const cleanup = registerBridge(ws, queryClient);
      ws.emit({
        type: "notification",
        action,
        payload: {
          workspace_id: "workspace-1",
          task_id: "office-task-1",
          agent_profile_id: "agent-1",
          run_id: "run-1",
        },
      });

      expect(queryClient.getQueryState(qk.office.dashboard("workspace-1"))?.isInvalidated).toBe(
        true,
      );
      expect(queryClient.getQueryState(qk.office.agentSummary("agent-1", 14))?.isInvalidated).toBe(
        true,
      );
      expect(
        queryClient.getQueryState(qk.office.agentRuns("agent-1", { limit: 25 }))?.isInvalidated,
      ).toBe(true);
      expect(
        queryClient.getQueryState(qk.office.runDetail("agent-1", "run-1"))?.isInvalidated,
      ).toBe(true);
      expect(
        queryClient.getQueryState(qk.office.taskActivity("workspace-1", "office-task-1"))
          ?.isInvalidated,
      ).toBe(true);

      cleanup();
    }
  });

  it("invalidates exact task activity for comment and review events", () => {
    const events: Array<
      "office.comment.created" | "office.task.decision_recorded" | "office.task.review_requested"
    > = ["office.comment.created", "office.task.decision_recorded", "office.task.review_requested"];

    for (const action of events) {
      const ws = new FakeWebSocketClient();
      const queryClient = makeQueryClient();
      queryClient.setQueryData(qk.office.taskActivity("workspace-1", "office-task-1"), {
        activity: [],
      });

      const cleanup = registerBridge(ws, queryClient);
      ws.emit({
        type: "notification",
        action,
        payload: {
          workspace_id: "workspace-1",
          task_id: "office-task-1",
        },
      });

      expect(
        queryClient.getQueryState(qk.office.taskActivity("workspace-1", "office-task-1"))
          ?.isInvalidated,
      ).toBe(true);

      cleanup();
    }
  });

  it("invalidates agent route queries for routing events", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.office.agentRoute("agent-1"), { route: null });
    queryClient.setQueryData(qk.office.agentRoute("agent-2"), { route: null });

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "office.provider.health_changed",
      payload: {
        workspace_id: "workspace-1",
        provider_id: "claude-acp",
        state: "degraded",
      },
    });

    expect(queryClient.getQueryState(qk.office.agentRoute("agent-1"))?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(qk.office.agentRoute("agent-2"))?.isInvalidated).toBe(true);

    cleanup();
  });

  it("invalidates targeted agent route when route attempts carry an agent id", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.office.agentRoute("agent-1"), { route: null });
    queryClient.setQueryData(qk.office.agentRoute("agent-2"), { route: null });

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "office.route_attempt.appended",
      payload: {
        run_id: "run-1",
        agent_profile_id: "agent-1",
        attempt: {
          seq: 1,
          provider_id: "codex",
          tier: "balanced",
          outcome: "launched",
          started_at: "2026-06-23T00:00:00Z",
        },
      },
    });

    expect(queryClient.getQueryState(qk.office.agentRoute("agent-1"))?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(qk.office.agentRoute("agent-2"))?.isInvalidated).toBe(false);

    cleanup();
  });

  it("invalidates agent summaries and runs when session state changes", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.office.agentSummary("agent-1", 14), { summary: {} });
    queryClient.setQueryData(qk.office.agentRuns("agent-1", { limit: 25 }), { runs: [] });
    queryClient.setQueryData(qk.office.runs("workspace-1"), { runs: [] });
    queryClient.setQueryData(qk.office.dashboard("workspace-1"), {});

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "session.state_changed",
      payload: {
        session_id: "session-1",
        task_id: "task-1",
        old_state: "WAITING_FOR_INPUT",
        new_state: "RUNNING",
      },
    });

    expect(queryClient.getQueryState(qk.office.agentSummary("agent-1", 14))?.isInvalidated).toBe(
      true,
    );
    expect(
      queryClient.getQueryState(qk.office.agentRuns("agent-1", { limit: 25 }))?.isInvalidated,
    ).toBe(true);
    expect(queryClient.getQueryState(qk.office.runs("workspace-1"))?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(qk.office.dashboard("workspace-1"))?.isInvalidated).toBe(true);

    cleanup();
  });

  it("upserts session messages into the stable session message cache", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.session.messages("session-1"), {
      messages: [
        {
          id: "message-1",
          session_id: "session-1",
          task_id: "task-1",
          author_type: "agent",
          content: "Old",
          type: "message",
          created_at: "2026-06-23T00:00:00Z",
        },
      ],
      hasMore: false,
      oldestCursor: "message-1",
    });

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "session.message.updated",
      payload: {
        task_id: "task-1",
        session_id: "session-1",
        message_id: "message-1",
        author_type: "agent",
        content: "Updated",
        type: "message",
        created_at: "2026-06-23T00:00:00Z",
        updated_at: "2026-06-23T00:00:01Z",
      },
    });

    expect(queryClient.getQueryData(qk.session.messages("session-1"))).toMatchObject({
      messages: [{ id: "message-1", content: "Updated" }],
    });
    expect(getBridgeAuditRows()).toContainEqual(
      expect.objectContaining({
        action: "session.message.updated",
        sessionId: "session-1",
        status: "handled",
      }),
    );

    cleanup();
  });

  it("does not seed latest session messages from a single event when the cache is missing", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "session.message.added",
      payload: {
        task_id: "task-1",
        session_id: "session-1",
        message_id: "message-1",
        author_type: "agent",
        content: "Needs input",
        type: "message",
        created_at: "2026-06-23T00:00:00Z",
      },
    });

    expect(queryClient.getQueryData(qk.session.messages("session-1"))).toBeUndefined();
    expect(getBridgeAuditRows()).toContainEqual(
      expect.objectContaining({
        action: "session.message.added",
        sessionId: "session-1",
        status: "handled",
      }),
    );

    cleanup();
  });

  it("removes deleted session messages from query caches", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    const keptMessage = {
      id: "message-keep",
      session_id: "session-1",
      task_id: "task-1",
      author_type: "user",
      content: "Keep",
      type: "message",
      created_at: "2026-06-23T00:00:00Z",
    };
    const deletedMessage = {
      id: "message-delete",
      session_id: "session-1",
      task_id: "task-1",
      author_type: "agent",
      content: "Delete",
      type: "message",
      created_at: "2026-06-23T00:00:01Z",
    };
    queryClient.setQueryData(qk.session.messages("session-1"), {
      messages: [keptMessage, deletedMessage],
      hasMore: false,
      oldestCursor: "message-keep",
    });
    queryClient.setQueryData(qk.session.messagesPage("session-1"), {
      messages: [keptMessage, deletedMessage],
    });
    queryClient.setQueryData(qk.session.messagesInfinite("session-1"), {
      pages: [{ messages: [keptMessage, deletedMessage] }],
      pageParams: [undefined],
    });

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "session.message.deleted",
      payload: {
        task_id: "task-1",
        session_id: "session-1",
        message_id: "message-delete",
        author_type: "agent",
        content: "",
        type: "message",
        created_at: "2026-06-23T00:00:01Z",
      },
    });

    expect(queryClient.getQueryData(qk.session.messages("session-1"))).toMatchObject({
      messages: [expect.objectContaining({ id: "message-keep" })],
    });
    expect(queryClient.getQueryData(qk.session.messagesPage("session-1"))).toMatchObject({
      messages: [expect.objectContaining({ id: "message-keep" })],
    });
    expect(queryClient.getQueryData(qk.session.messagesInfinite("session-1"))).toMatchObject({
      pages: [{ messages: [expect.objectContaining({ id: "message-keep" })] }],
    });
    expect(getBridgeAuditRows()).toContainEqual(
      expect.objectContaining({
        action: "session.message.deleted",
        sessionId: "session-1",
        status: "handled",
      }),
    );

    cleanup();
  });

  it("patches session-by-id and invalidates task session lists on state changes", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.taskSession.byId("session-1"), {
      id: "session-1",
      task_id: "task-1",
      state: "RUNNING",
    });
    queryClient.setQueryData(qk.taskSession.byTask("task-1"), {
      sessions: [{ id: "session-1", state: "RUNNING" }],
    });

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "session.state_changed",
      payload: {
        task_id: "task-1",
        session_id: "session-1",
        old_state: "RUNNING",
        new_state: "WAITING_FOR_INPUT",
        updated_at: "2026-06-23T00:00:01Z",
      },
    });

    expect(queryClient.getQueryData(qk.taskSession.byId("session-1"))).toMatchObject({
      state: "WAITING_FOR_INPUT",
      updated_at: "2026-06-23T00:00:01Z",
    });
    expect(queryClient.getQueryData(qk.taskSession.byTask("task-1"))).toMatchObject({
      sessions: [{ id: "session-1", state: "WAITING_FOR_INPUT" }],
    });
    expect(queryClient.getQueryState(qk.taskSession.byTask("task-1"))?.isInvalidated).toBe(true);

    cleanup();
  });

  it("patches primary task card session state in workflow snapshot and task detail caches", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    queryClient.setQueryData(
      qk.workflows.snapshot("workflow-1"),
      workflowSnapshot([
        workflowSnapshotTask(),
        workflowSnapshotTask({
          id: toTaskId("task-2"),
          primary_session_id: toSessionId("session-other"),
          primary_session_state: "WAITING_FOR_INPUT",
        }),
      ]),
    );
    queryClient.setQueryData(qk.tasks.detail("task-1"), workflowSnapshotTask());

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "session.state_changed",
      payload: {
        task_id: "task-1",
        session_id: "session-1",
        old_state: "WAITING_FOR_INPUT",
        new_state: "RUNNING",
        updated_at: "2026-06-24T00:01:00Z",
      },
    });

    expect(queryClient.getQueryData<WorkflowSnapshot>(qk.workflows.snapshot("workflow-1"))).toEqual(
      expect.objectContaining({
        tasks: [
          expect.objectContaining({
            id: "task-1",
            primary_session_id: "session-1",
            primary_session_state: "RUNNING",
          }),
          expect.objectContaining({
            id: "task-2",
            primary_session_state: "WAITING_FOR_INPUT",
          }),
        ],
      }),
    );
    expect(queryClient.getQueryData(qk.tasks.detail("task-1"))).toMatchObject({
      primary_session_state: "RUNNING",
    });

    cleanup();
  });

  it("does not create a full install-job list from a single install event", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "agent.install.started",
      payload: {
        job_id: "job-1",
        agent_name: "codex",
        status: "running",
        started_at: "2026-06-24T00:00:00Z",
      },
    });

    expect(queryClient.getQueryData(qk.settings.installJob("job-1"))).toMatchObject({
      job_id: "job-1",
      status: "running",
    });
    expect(queryClient.getQueryData(qk.settings.installJobs())).toBeUndefined();

    cleanup();
  });

  it("patches and invalidates an existing install-job list", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.settings.installJobs(), {
      jobs: [
        {
          job_id: "job-1",
          agent_name: "codex",
          status: "running",
          started_at: "2026-06-24T00:00:00Z",
        },
        {
          job_id: "job-2",
          agent_name: "claude",
          status: "running",
          started_at: "2026-06-24T00:00:00Z",
        },
      ],
    });

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "agent.install.finished",
      payload: {
        job_id: "job-1",
        agent_name: "codex",
        status: "succeeded",
        started_at: "2026-06-24T00:00:00Z",
        finished_at: "2026-06-24T00:01:00Z",
      },
    });

    expect(queryClient.getQueryData(qk.settings.installJobs())).toMatchObject({
      jobs: [
        { job_id: "job-1", status: "succeeded" },
        { job_id: "job-2", status: "running" },
      ],
    });
    expect(queryClient.getQueryState(qk.settings.installJobs())?.isInvalidated).toBe(true);

    cleanup();
  });

  it("preserves available-agent tools when availability events omit tools", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.settings.availableAgents(), {
      agents: [{ name: "codex", available: true }],
      tools: [{ name: "codex", installed: true }],
      total: 1,
    });

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "agent.available.updated",
      payload: {
        agents: [{ name: "codex", available: false }],
      },
    });

    expect(queryClient.getQueryData(qk.settings.availableAgents())).toEqual({
      agents: [{ name: "codex", available: false }],
      tools: [{ name: "codex", installed: true }],
      total: 1,
    });

    cleanup();
  });

  it("does not seed available-agent snapshots from partial availability events", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "agent.available.updated",
      payload: {
        agents: [{ name: "codex", available: false }],
      },
    });

    expect(queryClient.getQueryData(qk.settings.availableAgents())).toBeUndefined();

    cleanup();
  });

  it("keeps session turns active id in the query cache", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.session.turns("session-1"), { turns: [], activeTurnId: null });

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "session.turn.started",
      payload: {
        id: "turn-1",
        task_id: "task-1",
        session_id: "session-1",
        started_at: "2026-06-23T00:00:00Z",
        created_at: "2026-06-23T00:00:00Z",
        updated_at: "2026-06-23T00:00:00Z",
      },
    });
    ws.emit({
      type: "notification",
      action: "session.turn.completed",
      payload: {
        id: "turn-1",
        task_id: "task-1",
        session_id: "session-1",
        started_at: "2026-06-23T00:00:00Z",
        completed_at: "2026-06-23T00:00:03Z",
        created_at: "2026-06-23T00:00:00Z",
        updated_at: "2026-06-23T00:00:03Z",
      },
    });

    expect(queryClient.getQueryData(qk.session.turns("session-1"))).toMatchObject({
      activeTurnId: null,
      turns: [{ id: "turn-1", completed_at: "2026-06-23T00:00:03Z" }],
    });

    cleanup();
  });

  it("does not create a full turns list from a single live turn event", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "session.turn.started",
      payload: {
        id: "turn-1",
        task_id: "task-1",
        session_id: "session-1",
        started_at: "2026-06-23T00:00:00Z",
        created_at: "2026-06-23T00:00:00Z",
        updated_at: "2026-06-23T00:00:00Z",
      },
    });

    expect(queryClient.getQueryData(qk.session.turns("session-1"))).toBeUndefined();

    cleanup();
  });

  it("patches queue status and task-plan query caches from session events", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.session.queue("session-1"), { entries: [], count: 0, max: 10 });
    queryClient.setQueryData(qk.taskPlan.detail("task-1"), null);
    queryClient.setQueryData(qk.taskPlan.revisions("task-1"), []);

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "message.queue.status_changed",
      payload: {
        session_id: "session-1",
        entries: [
          {
            id: "queue-1",
            session_id: "session-1",
            task_id: "task-1",
            content: "Queued",
            plan_mode: false,
            queued_at: "2026-06-23T00:00:00Z",
          },
        ],
        count: 1,
        max: 10,
      },
    });
    ws.emit({
      type: "notification",
      action: "task.plan.updated",
      payload: {
        id: "plan-1",
        task_id: "task-1",
        title: "Plan",
        content: "Updated plan",
        created_by: "agent",
        created_at: "2026-06-23T00:00:00Z",
        updated_at: "2026-06-23T00:00:01Z",
      },
    });
    queryClient.setQueryData(qk.taskPlan.revision("task-1", "revision-1"), {
      id: "revision-1",
      task_id: "task-1",
      content: "stale content",
    });
    ws.emit({
      type: "notification",
      action: "task.plan.revision.created",
      payload: {
        id: "revision-1",
        task_id: "task-1",
        revision_number: 1,
        title: "Plan",
        author_kind: "agent",
        author_name: "Agent",
        created_at: "2026-06-23T00:00:01Z",
        updated_at: "2026-06-23T00:00:01Z",
      },
    });

    expect(queryClient.getQueryData(qk.session.queue("session-1"))).toMatchObject({
      count: 1,
      entries: [{ id: "queue-1", content: "Queued" }],
    });
    expect(queryClient.getQueryData(qk.taskPlan.detail("task-1"))).toMatchObject({
      id: "plan-1",
      content: "Updated plan",
    });
    expect(queryClient.getQueryData(qk.taskPlan.revisions("task-1"))).toMatchObject([
      { id: "revision-1", revision_number: 1 },
    ]);
    expect(queryClient.getQueryState(qk.taskPlan.revisions("task-1"))).toMatchObject({
      isInvalidated: true,
    });
    expect(queryClient.getQueryState(qk.taskPlan.revision("task-1", "revision-1"))).toMatchObject({
      isInvalidated: true,
    });

    cleanup();
  });

  it("does not create a full revision list from a single task-plan revision event", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "task.plan.revision.created",
      payload: {
        id: "revision-1",
        task_id: "task-1",
        revision_number: 1,
        title: "Plan",
        author_kind: "agent",
        author_name: "Agent",
        created_at: "2026-06-23T00:00:01Z",
        updated_at: "2026-06-23T00:00:01Z",
      },
    });

    expect(queryClient.getQueryData(qk.taskPlan.revisions("task-1"))).toBeUndefined();

    cleanup();
  });

  it("patches session runtime query caches from runtime events", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.taskSession.byId("session-1"), {
      id: "session-1",
      task_id: "task-1",
      task_environment_id: "env-1",
      repository_id: "repo-1",
    });
    queryClient.setQueryData(qk.sessionRuntime.gitStatus("env-1"), { byRepo: {} });
    queryClient.setQueryData(qk.sessionRuntime.commits("env-1"), []);

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "session.git.event",
      payload: {
        type: "status_update",
        session_id: "session-1",
        timestamp: "2026-06-23T00:00:00Z",
        status: {
          branch: "feature",
          remote_branch: "origin/feature",
          modified: ["a.ts"],
          added: [],
          deleted: [],
          untracked: [],
          renamed: [],
          ahead: 1,
          behind: 0,
          files: {
            "a.ts": { path: "a.ts", status: "modified", staged: false },
          },
          repository_name: "repo",
        },
      },
    });
    ws.emit({
      type: "notification",
      action: "session.git.event",
      payload: {
        type: "commit_created",
        session_id: "session-1",
        timestamp: "2026-06-23T00:00:01Z",
        commit: {
          id: "commit-1",
          commit_sha: "abc",
          parent_sha: "base",
          commit_message: "commit",
          author_name: "A",
          author_email: "a@example.com",
          files_changed: 1,
          insertions: 2,
          deletions: 1,
          committed_at: "2026-06-23T00:00:01Z",
        },
      },
    });
    ws.emit({
      type: "notification",
      action: "executor.prepare.progress",
      payload: {
        session_id: "session-1",
        step_index: 0,
        step_name: "Clone",
        status: "running",
      },
    });
    ws.emit({
      type: "notification",
      action: "session.models_updated",
      payload: {
        task_id: "task-1",
        session_id: "session-1",
        agent_id: "agent-1",
        current_model_id: "gpt-5",
        models: [{ model_id: "gpt-5", name: "GPT-5", usage_multiplier: "1" }],
        config_options: [
          {
            type: "select",
            id: "effort",
            name: "Effort",
            description: "Controls reasoning effort",
            current_value: "medium",
            options: [{ value: "medium", name: "Medium", description: "Balanced reasoning" }],
          },
        ],
        timestamp: "2026-06-23T00:00:02Z",
      },
    });
    ws.emit({
      type: "notification",
      action: "session.agentctl_ready",
      timestamp: "2026-06-23T00:00:03Z",
      payload: {
        task_id: "task-1",
        session_id: "session-1",
        task_environment_id: "env-1",
        agent_execution_id: "exec-1",
        worktree_id: "worktree-1",
        worktree_path: "/tmp/kandev/worktrees/worktree-1",
        worktree_branch: "feature/session",
      },
    });
    ws.emit({
      type: "notification",
      action: "session.process.status",
      payload: {
        session_id: "session-1",
        process_id: "process-1",
        kind: "dev",
        status: "running",
        timestamp: "2026-06-23T00:00:04Z",
      },
    });

    expect(queryClient.getQueryData(qk.sessionRuntime.gitStatus("env-1"))).toMatchObject({
      latest: { branch: "feature" },
      byRepo: { repo: { branch: "feature" } },
    });
    expect(queryClient.getQueryData(qk.sessionRuntime.commits("env-1"))).toMatchObject([
      { id: "commit-1", commit_sha: "abc" },
    ]);
    expect(queryClient.getQueryData(qk.sessionRuntime.prepare("session-1"))).toMatchObject({
      status: "preparing",
      steps: [{ name: "Clone", status: "running" }],
    });
    expect(queryClient.getQueryData(qk.sessionRuntime.models("session-1"))).toMatchObject({
      currentModelId: "gpt-5",
      models: [{ modelId: "gpt-5", name: "GPT-5" }],
      configOptions: [
        {
          id: "effort",
          description: "Controls reasoning effort",
          options: [{ value: "medium", description: "Balanced reasoning" }],
        },
      ],
    });
    expect(queryClient.getQueryData(qk.sessionRuntime.agentctl("session-1"))).toMatchObject({
      status: "ready",
      agentExecutionId: "exec-1",
    });
    expect(queryClient.getQueryData(qk.sessionRuntime.worktrees("session-1"))).toEqual([
      {
        id: "worktree-1",
        sessionId: "session-1",
        repositoryId: "repo-1",
        path: "/tmp/kandev/worktrees/worktree-1",
        branch: "feature/session",
      },
    ]);
    expect(queryClient.getQueryData(qk.sessionRuntime.processes("session-1"))).toMatchObject({
      devProcessId: "process-1",
      processesById: { "process-1": { status: "running" } },
    });
    expect(getBridgeAuditRows()).toContainEqual(
      expect.objectContaining({
        action: "session.models_updated",
        sessionId: "session-1",
        status: "handled",
      }),
    );

    cleanup();
  });

  it("does not create partial task session detail rows from agentctl events", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "session.agentctl_ready",
      timestamp: "2026-06-23T00:00:03Z",
      payload: {
        task_id: "task-1",
        session_id: "session-1",
        task_environment_id: "env-1",
        agent_execution_id: "exec-1",
        worktree_id: "worktree-1",
        worktree_path: "/tmp/kandev/worktrees/worktree-1",
        worktree_branch: "feature/session",
      },
    });

    expect(queryClient.getQueryData(qk.taskSession.byId("session-1"))).toBeUndefined();
    expect(queryClient.getQueryData(qk.sessionRuntime.agentctl("session-1"))).toMatchObject({
      status: "ready",
      agentExecutionId: "exec-1",
    });
    expect(queryClient.getQueryData(qk.sessionRuntime.worktrees("session-1"))).toEqual([
      {
        id: "worktree-1",
        sessionId: "session-1",
        repositoryId: undefined,
        path: "/tmp/kandev/worktrees/worktree-1",
        branch: "feature/session",
      },
    ]);

    cleanup();
  });

  it("preserves existing worktree metadata when agentctl ready events omit optional fields", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.sessionRuntime.worktrees("session-1"), [
      {
        id: "worktree-1",
        sessionId: "session-1",
        repositoryId: "repo-1",
        path: "/tmp/kandev/worktrees/worktree-1",
        branch: "feature/session",
      },
    ]);

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "session.agentctl_ready",
      timestamp: "2026-06-23T00:00:03Z",
      payload: {
        task_id: "task-1",
        session_id: "session-1",
        task_environment_id: "env-1",
        agent_execution_id: "exec-1",
        worktree_id: "worktree-1",
      },
    });

    expect(queryClient.getQueryData(qk.sessionRuntime.worktrees("session-1"))).toEqual([
      {
        id: "worktree-1",
        sessionId: "session-1",
        repositoryId: "repo-1",
        path: "/tmp/kandev/worktrees/worktree-1",
        branch: "feature/session",
      },
    ]);

    cleanup();
  });

  it("seeds the primary worktree when an uncached sibling worktree becomes ready", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.taskSession.byId("session-1"), {
      id: "session-1",
      task_id: "task-1",
      task_environment_id: "env-1",
      repository_id: "repo-1",
      worktree_id: "primary-worktree",
      worktree_path: "/tmp/kandev/worktrees/primary-worktree",
      worktree_branch: "main",
    });

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "session.agentctl_ready",
      timestamp: "2026-06-23T00:00:03Z",
      payload: {
        task_id: "task-1",
        session_id: "session-1",
        task_environment_id: "env-1",
        agent_execution_id: "exec-1",
        worktree_id: "sibling-worktree",
        worktree_path: "/tmp/kandev/worktrees/sibling-worktree",
        worktree_branch: "feature/sibling",
      },
    });

    expect(queryClient.getQueryData(qk.taskSession.byId("session-1"))).toMatchObject({
      worktree_id: "primary-worktree",
      worktree_path: "/tmp/kandev/worktrees/primary-worktree",
      worktree_branch: "main",
    });
    expect(queryClient.getQueryData(qk.sessionRuntime.worktrees("session-1"))).toEqual([
      {
        id: "primary-worktree",
        sessionId: "session-1",
        repositoryId: "repo-1",
        path: "/tmp/kandev/worktrees/primary-worktree",
        branch: "main",
      },
      {
        id: "sibling-worktree",
        sessionId: "session-1",
        repositoryId: "repo-1",
        path: "/tmp/kandev/worktrees/sibling-worktree",
        branch: "feature/sibling",
      },
    ]);

    cleanup();
  });

  it("stales session worktree queries when an agentctl event arrives before session detail", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();

    const cleanup = registerBridge(ws, queryClient);
    ws.emit({
      type: "notification",
      action: "session.agentctl_ready",
      timestamp: "2026-06-23T00:00:03Z",
      payload: {
        task_id: "task-1",
        session_id: "session-1",
        task_environment_id: "env-1",
        agent_execution_id: "exec-1",
        worktree_id: "sibling-worktree",
        worktree_path: "/tmp/kandev/worktrees/sibling-worktree",
        worktree_branch: "feature/sibling",
      },
    });

    expect(queryClient.getQueryData(qk.sessionRuntime.worktrees("session-1"))).toEqual([
      expect.objectContaining({ id: "sibling-worktree" }),
    ]);
    expect(queryClient.getQueryState(qk.sessionRuntime.worktrees("session-1"))?.isInvalidated).toBe(
      true,
    );

    cleanup();
  });

  it("does not record audit rows when E2E exposure is disabled", () => {
    delete (window as BridgeWindow).__KANDEV_E2E_EXPOSE_STORE__;
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.tasks.detail("task-1"), { id: "task-1", title: "Old title" });

    const cleanup = registerBridge(ws, queryClient);
    ws.emit(taskUpdated({ title: "Still patched" }));

    expect(queryClient.getQueryData(qk.tasks.detail("task-1"))).toMatchObject({
      title: "Still patched",
    });
    expect(getBridgeAuditRows()).toEqual([]);
    expect((window as BridgeWindow).__kandev_bridge_audit__).toBeUndefined();

    cleanup();
  });

  it("unregisters bridge handlers and envelope audit listeners", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.tasks.detail("task-1"), { id: "task-1", title: "Old title" });

    const cleanup = registerBridge(ws, queryClient);
    cleanup();

    ws.emit(taskUpdated({ title: "Ignored" }));
    ws.emit({
      id: "response-1",
      type: "response",
      action: "session.subscribe",
      payload: { session_id: "session-1" },
    });

    expect(queryClient.getQueryData(qk.tasks.detail("task-1"))).toMatchObject({
      title: "Old title",
    });
    expect(getBridgeAuditRows()).toEqual([]);
  });
});
