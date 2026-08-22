import { describe, expect, it, vi } from "vitest";
import {
  taskId as toTaskId,
  workflowId as toWorkflowId,
  workspaceId as toWorkspaceId,
  type Repository,
  type Task,
} from "@/lib/types/http";
import type { KanbanState } from "@/lib/state/slices";
import {
  buildArchivedValue,
  buildDebugEntries,
  buildTaskFromKanban,
  hasResolvedTaskDetails,
  resolveEffectiveTask,
  resolveTaskContentState,
  resolveTaskProps,
  syncActiveTaskSession,
} from "./task-page-content-helpers";

type KanbanTask = KanbanState["tasks"][number];
const ARCHIVED_AT = "2026-07-19T00:00:00Z";

function makeArchivedTaskDetails(overrides: Partial<Task> = {}): Task {
  return {
    id: "task-1",
    title: "Archived task",
    description: "",
    workflow_step_id: "step-1",
    position: 0,
    state: "TODO",
    workspace_id: "ws-1",
    workflow_id: "wf-1",
    priority: "medium",
    repositories: [],
    created_at: "",
    updated_at: ARCHIVED_AT,
    archived_at: ARCHIVED_AT,
    ...overrides,
  } as Task;
}

function makeKanbanTask(overrides: Partial<KanbanTask> = {}): KanbanTask {
  return {
    id: "task-1",
    title: "Restored task",
    workflowStepId: "step-1",
    position: 0,
    state: "TODO",
    ...overrides,
  } as KanbanTask;
}

function baseParams(overrides: Partial<Parameters<typeof buildDebugEntries>[0]> = {}) {
  return {
    connectionStatus: "connected",
    task: null,
    effectiveSessionId: "s1",
    taskSessionState: "RUNNING",
    isAgentWorking: true,
    resumptionState: "idle",
    resumptionError: null,
    agentctlStatus: { status: "ready", isReady: true },
    previewOpen: false,
    previewStage: "closed",
    previewUrl: "",
    devProcessId: undefined,
    devProcessStatus: null,
    ...overrides,
  };
}

describe("buildDebugEntries", () => {
  it("includes active session ACP metadata", () => {
    const entries = buildDebugEntries(
      baseParams({
        activeSessionMetadata: {
          acp: {
            session_id: "acp-1",
            title: "List files",
            updated_at: "2026-06-13T19:37:46Z",
            meta: { cursor: { requestId: "req-1" } },
          },
        },
      }),
    );

    expect(entries.acp_session_id).toBe("acp-1");
    expect(entries).not.toHaveProperty("acp_session_title");
    expect(entries.acp_session_updated_at).toBe("2026-06-13T19:37:46Z");
    expect(entries.acp_meta).toEqual({ cursor: { requestId: "req-1" } });
  });
});

describe("resolveTaskProps", () => {
  it("exposes linked GitHub issue metadata for the top bar", () => {
    const props = resolveTaskProps(
      {
        id: "task-1",
        title: "Link issue",
        metadata: {
          issue_url: "https://github.com/kdlbs/kandev/issues/1470",
          issue_number: 1470,
        },
      } as unknown as Task,
      null,
    );

    expect(props.issueUrl).toBe("https://github.com/kdlbs/kandev/issues/1470");
    expect(props.issueNumber).toBe(1470);
  });

  it("labels the repository by its provider slug, not its local clone path", () => {
    const props = resolveTaskProps(
      { id: "task-1", title: "Any" } as unknown as Task,
      {
        id: "repo-1",
        name: "kandev",
        local_path: "/home/dev/src/kandev",
        provider_owner: "kdlbs",
        provider_name: "kandev",
      } as unknown as Repository,
    );

    expect(props.repositoryLabel).toBe("kdlbs/kandev");
  });

  it("falls back to the repository name when no provider owns it", () => {
    const props = resolveTaskProps(
      { id: "task-1", title: "Any" } as unknown as Task,
      {
        id: "repo-1",
        name: "scratchpad",
        local_path: "/home/dev/src/scratchpad",
      } as unknown as Repository,
    );

    expect(props.repositoryLabel).toBe("scratchpad");
  });

  it("has no repository label for a task with no repository", () => {
    const props = resolveTaskProps({ id: "task-1", title: "Any" } as unknown as Task, null);

    expect(props.repositoryLabel).toBeNull();
  });
});

describe("buildArchivedValue repository identity", () => {
  // The archived row renders this value, so a local clone path here would put
  // "/home/dev/src/kandev" in the sidebar and give archived tasks a different
  // repository grouping key from every ordinary task.
  it("carries the provider slug, not the local clone path", () => {
    const value = buildArchivedValue(
      { id: "task-1", title: "Any", archived_at: ARCHIVED_AT } as unknown as Task,
      {
        id: "repo-1",
        name: "kandev",
        local_path: "/home/dev/src/kandev",
        provider_owner: "kdlbs",
        provider_name: "kandev",
      } as unknown as Repository,
    );

    expect(value.archivedTaskRepositoryLabel).toBe("kdlbs/kandev");
  });

  it("leaves the label unset for a task that is not archived", () => {
    const value = buildArchivedValue(
      { id: "task-1", title: "Any" } as unknown as Task,
      {
        id: "repo-1",
        name: "kandev",
        local_path: "/home/dev/src/kandev",
        provider_owner: "kdlbs",
        provider_name: "kandev",
      } as unknown as Repository,
    );

    expect(value.archivedTaskRepositoryLabel).toBeUndefined();
  });
});

describe("resolveTaskContentState", () => {
  it("keeps showing the loading state until the component mounts", () => {
    expect(
      resolveTaskContentState({
        isMounted: false,
        hasTask: false,
        hasTaskLoadError: true,
      }),
    ).toBe("loading");
  });

  it("surfaces task load failures after mount", () => {
    expect(
      resolveTaskContentState({
        isMounted: true,
        hasTask: false,
        hasTaskLoadError: true,
      }),
    ).toBe("error");
  });

  it("surfaces task load failures even when a placeholder task exists", () => {
    expect(
      resolveTaskContentState({
        isMounted: true,
        hasTask: true,
        hasTaskLoadError: true,
      }),
    ).toBe("error");
  });

  it("treats a resolved task as ready", () => {
    expect(
      resolveTaskContentState({
        isMounted: true,
        hasTask: true,
        hasTaskLoadError: false,
      }),
    ).toBe("ready");
  });
});

describe("hasResolvedTaskDetails", () => {
  it("returns true when fetched details match the effective task", () => {
    expect(
      hasResolvedTaskDetails({
        effectiveTaskId: "task-1",
        taskDetailsId: "task-1",
        initialTaskId: null,
      }),
    ).toBe(true);
  });

  it("returns true when SSR task details match the effective task", () => {
    expect(
      hasResolvedTaskDetails({
        effectiveTaskId: "task-1",
        taskDetailsId: null,
        initialTaskId: "task-1",
      }),
    ).toBe(true);
  });

  it("returns false for kanban-only placeholder tasks", () => {
    expect(
      hasResolvedTaskDetails({
        effectiveTaskId: "task-1",
        taskDetailsId: "task-2",
        initialTaskId: null,
      }),
    ).toBe(false);
  });

  it("returns false when there is no effective task", () => {
    expect(
      hasResolvedTaskDetails({
        effectiveTaskId: null,
        taskDetailsId: "task-1",
        initialTaskId: "task-1",
      }),
    ).toBe(false);
  });
});

describe("syncActiveTaskSession", () => {
  it("restores the initial session without creating a user pin", () => {
    const setActiveSessionAuto = vi.fn();
    const setActiveTask = vi.fn();

    const applied = syncActiveTaskSession({
      initialTaskId: "task-1",
      fallbackTaskId: null,
      initialSessionId: "session-1",
      activeTaskId: null,
      previousRouteTaskId: undefined,
      setActiveSessionAuto,
      setActiveTask,
    });

    expect(applied).toBe(true);
    expect(setActiveSessionAuto).toHaveBeenCalledWith("task-1", "session-1");
    expect(setActiveTask).not.toHaveBeenCalled();
  });

  it("falls back to selecting the task when there is no initial session", () => {
    const setActiveSessionAuto = vi.fn();
    const setActiveTask = vi.fn();

    const applied = syncActiveTaskSession({
      initialTaskId: "task-1",
      fallbackTaskId: null,
      initialSessionId: null,
      activeTaskId: null,
      previousRouteTaskId: undefined,
      setActiveSessionAuto,
      setActiveTask,
    });

    expect(applied).toBe(true);
    expect(setActiveTask).toHaveBeenCalledWith("task-1");
    expect(setActiveSessionAuto).not.toHaveBeenCalled();
  });

  it("applies a changed route over the previous active task", () => {
    const setActiveSessionAuto = vi.fn();
    const setActiveTask = vi.fn();

    const applied = syncActiveTaskSession({
      initialTaskId: "task-2",
      fallbackTaskId: null,
      initialSessionId: "session-2",
      activeTaskId: "task-1",
      previousRouteTaskId: "task-1",
      setActiveSessionAuto,
      setActiveTask,
    });

    expect(applied).toBe(true);
    expect(setActiveSessionAuto).toHaveBeenCalledWith("task-2", "session-2");
    expect(setActiveTask).not.toHaveBeenCalled();
  });

  it("adopts a session that arrives for the current route", () => {
    const setActiveSessionAuto = vi.fn();
    const setActiveTask = vi.fn();

    const applied = syncActiveTaskSession({
      initialTaskId: "task-1",
      fallbackTaskId: null,
      initialSessionId: "session-1",
      activeTaskId: "task-1",
      previousRouteTaskId: "task-1",
      setActiveSessionAuto,
      setActiveTask,
    });

    expect(applied).toBe(true);
    expect(setActiveSessionAuto).toHaveBeenCalledWith("task-1", "session-1");
    expect(setActiveTask).not.toHaveBeenCalled();
  });

  it("does not restore an unchanged route over an in-place sibling selection", () => {
    const setActiveSessionAuto = vi.fn();
    const setActiveTask = vi.fn();
    const applied = syncActiveTaskSession({
      initialTaskId: "missing-task",
      fallbackTaskId: null,
      initialSessionId: "sibling-session",
      activeTaskId: "sibling-task",
      previousRouteTaskId: "missing-task",
      setActiveSessionAuto,
      setActiveTask,
    });

    expect(applied).toBe(false);
    expect(setActiveSessionAuto).not.toHaveBeenCalled();
    expect(setActiveTask).not.toHaveBeenCalled();
  });
});

describe("resolveEffectiveTask archived state", () => {
  it("preserves a non-default priority for kanban-only tasks", () => {
    const resolved = buildTaskFromKanban(makeKanbanTask({ priority: "high" }));

    expect(resolved.priority).toBe("high");
  });

  it("builds a kanban-only task with its metadata", () => {
    const metadata = { port_forwarding_enabled: true };
    const resolved = resolveEffectiveTask(null, null, makeKanbanTask({ metadata }), "task-1");

    expect(resolved?.metadata).toEqual(metadata);
  });

  it("uses live kanban metadata while preserving base metadata when omitted", () => {
    const base = makeArchivedTaskDetails({ metadata: { port_forwarding_enabled: false } });
    const enabled = resolveEffectiveTask(
      base,
      null,
      makeKanbanTask({ metadata: { port_forwarding_enabled: true } }),
      "task-1",
    );
    expect(enabled?.metadata).toEqual({ port_forwarding_enabled: true });

    const omitted = resolveEffectiveTask(base, null, makeKanbanTask(), "task-1");
    expect(omitted?.metadata).toEqual({ port_forwarding_enabled: false });
  });

  it("keeps fetched archived state when a stale matching kanban card remains", () => {
    const taskDetails = makeArchivedTaskDetails();
    const kanbanTask = makeKanbanTask({ updatedAt: "2026-07-18T00:00:00Z" });

    const resolved = resolveEffectiveTask(taskDetails, null, kanbanTask, "task-1");

    expect(resolved?.archived_at).toBe(ARCHIVED_AT);
    expect(buildArchivedValue(resolved, null).isArchived).toBe(true);
  });

  it("clears fetched archived state when a matching kanban card is newer", () => {
    const taskDetails = makeArchivedTaskDetails();
    const kanbanTask = makeKanbanTask({ updatedAt: "2026-07-20T00:00:00Z" });

    const resolved = resolveEffectiveTask(taskDetails, null, kanbanTask, "task-1");

    expect(resolved?.archived_at).toBeNull();
    expect(buildArchivedValue(resolved, null).isArchived).toBe(false);
  });

  it("keeps archived_at when the task is absent from the kanban (still archived)", () => {
    const taskDetails = makeArchivedTaskDetails();

    const resolved = resolveEffectiveTask(taskDetails, null, null, "task-1");

    expect(resolved?.archived_at).toBe(ARCHIVED_AT);
    expect(buildArchivedValue(resolved, null).isArchived).toBe(true);
  });

  it("prefers live kanban title/state while preserving base-only fields", () => {
    const taskDetails = makeArchivedTaskDetails({ archived_at: null });
    const kanbanTask = makeKanbanTask({ title: "Live title", state: "IN_PROGRESS" });

    const resolved = resolveEffectiveTask(taskDetails, null, kanbanTask, "task-1");

    expect(resolved?.title).toBe("Live title");
    expect(resolved?.state).toBe("IN_PROGRESS");
    expect(resolved?.workspace_id).toBe("ws-1");
  });

  it("does not copy IDs from rejected task details into a kanban-only placeholder", () => {
    const unrelatedTask = makeArchivedTaskDetails({
      id: toTaskId("other-task"),
      workspace_id: toWorkspaceId("other-workspace"),
      workflow_id: toWorkflowId("other-workflow"),
    });
    const kanbanTask = makeKanbanTask({ id: toTaskId("task-1") });

    const resolved = resolveEffectiveTask(unrelatedTask, null, kanbanTask, "task-1");

    expect(resolved?.workspace_id).toBe("");
    expect(resolved?.workflow_id).toBe("");
  });
});
