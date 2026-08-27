import { describe, it, expect } from "vitest";
import { toKanbanTask, preserveOmittedExecutorFields, type TaskLike } from "./map-task";

/**
 * Parity matrix: the HTTP DTO and the WS payload describe the same task from
 * two shapes. Both must produce the same KanbanTask. If someone changes the
 * backend publisher (or adds a metadata-derived field to one side only), a
 * test here fails loudly — instead of silently diverging until a sidebar bug
 * surfaces.
 */

const BASE_SCALARS = {
  workflow_step_id: "step-1",
  title: "Ship the thing",
  description: "Do it well",
  position: 3,
  state: "TODO" as const,
  priority: "critical",
  is_ephemeral: false,
  created_at: "2026-04-22T10:00:00Z",
  updated_at: "2026-04-22T10:05:00Z",
};

function httpDTO(overrides: Partial<TaskLike> = {}): TaskLike {
  return {
    id: "task-1",
    workspace_id: "ws-1",
    workflow_id: "wf-1",
    ...BASE_SCALARS,
    repositories: [{ repository_id: "repo-a" }],
    primary_session_id: "session-p",
    primary_session_state: "RUNNING",
    session_count: 2,
    review_status: "pending",
    primary_executor_id: "exec-1",
    primary_executor_type: "local_docker",
    primary_executor_name: "Docker",
    is_remote_executor: false,
    parent_id: "parent-1",
    metadata: null,
    ...overrides,
  } as TaskLike;
}

function wsPayload(overrides: Partial<TaskLike> = {}): TaskLike {
  return {
    task_id: "task-1",
    workspace_id: "ws-1",
    workflow_id: "wf-1",
    ...BASE_SCALARS,
    // Backend WS task.updated emits both the primary repository_id *and* the
    // full repositories array so multi-repo state survives WS-driven updates.
    repository_id: "repo-a",
    repositories: [{ repository_id: "repo-a" }],
    primary_session_id: "session-p",
    primary_session_state: "RUNNING",
    session_count: 2,
    review_status: "pending",
    primary_executor_id: "exec-1",
    primary_executor_type: "local_docker",
    primary_executor_name: "Docker",
    is_remote_executor: false,
    parent_id: "parent-1",
    metadata: null,
    ...overrides,
  } as TaskLike;
}

describe("toKanbanTask — HTTP DTO / WS payload parity", () => {
  it("carries workspace identity and archived state through both task shapes", () => {
    const archivedAt = "2026-08-04T10:00:00Z";
    const http = toKanbanTask(httpDTO({ archived_at: archivedAt } as Partial<TaskLike>));
    const ws = toKanbanTask(wsPayload({ archived_at: archivedAt } as Partial<TaskLike>));

    expect(http.workspaceId).toBe("ws-1");
    expect(http.isArchived).toBe(true);
    expect(ws.workspaceId).toBe("ws-1");
    expect(ws.isArchived).toBe(true);
  });
  it("plain task: both shapes produce identical KanbanTask", () => {
    expect(toKanbanTask(httpDTO())).toEqual(toKanbanTask(wsPayload()));
  });

  it("carries task metadata through both shapes", () => {
    const metadata = { port_forwarding_enabled: true, unrelated: "preserve" };
    const http = toKanbanTask(httpDTO({ metadata }));
    const ws = toKanbanTask(wsPayload({ metadata }));

    expect(http.metadata).toEqual(metadata);
    expect(ws.metadata).toEqual(metadata);
    expect(ws).toEqual(http);
  });

  it("PR review task: review_watch_id flags isPRReview from both shapes", () => {
    const metadata = { review_watch_id: "watch-123" };
    const out = toKanbanTask(httpDTO({ metadata }));
    expect(out.isPRReview).toBe(true);
    expect(out.isIssueWatch).toBe(false);
    expect(toKanbanTask(wsPayload({ metadata }))).toEqual(out);
  });

  it("issue watch task: issue_watch_id + issue_url/issue_number mirrored on both shapes", () => {
    const metadata = {
      issue_watch_id: "watch-9",
      issue_url: "https://github.com/owner/repo/issues/42",
      issue_number: 42,
    };
    const out = toKanbanTask(httpDTO({ metadata }));
    expect(out.isIssueWatch).toBe(true);
    expect(out.issueUrl).toBe("https://github.com/owner/repo/issues/42");
    expect(out.issueNumber).toBe(42);
    expect(toKanbanTask(wsPayload({ metadata }))).toEqual(out);
  });

  it("repositoryId comes from nested HTTP repositories[0] or flat WS repository_id", () => {
    expect(toKanbanTask(httpDTO()).repositoryId).toBe("repo-a");
    expect(toKanbanTask(wsPayload()).repositoryId).toBe("repo-a");
  });

  it("preserves branch policy snapshot fields through HTTP and WS mapping", () => {
    const repositories = [
      {
        repository_id: "repo-a",
        base_branch: "main",
        checkout_branch: "feature/ship-it",
        branch_policy_id: "policy-a",
        branch_policy_name: "Feature branches",
        branch_policy_base_branch: "main",
        branch_policy_branch_template: "feature/{title}-{suffix}",
        branch_policy_pull_request_target: "develop",
      },
    ];
    const http = toKanbanTask(httpDTO({ repositories }));
    const ws = toKanbanTask(wsPayload({ repositories }));

    expect(http.repositories?.[0]).toMatchObject({
      branch_policy_id: "policy-a",
      branch_policy_name: "Feature branches",
      branch_policy_base_branch: "main",
      branch_policy_branch_template: "feature/{title}-{suffix}",
      branch_policy_pull_request_target: "develop",
    });
    expect(ws.repositories).toEqual(http.repositories);
  });
});

describe("toKanbanTask — pending and status fields", () => {
  it("maps primary session pending action from HTTP and WS shapes", () => {
    const pendingAction = {
      primary_session_pending_action: "clarification",
    } as Record<string, unknown> as Partial<TaskLike>;
    const http = toKanbanTask(httpDTO(pendingAction));
    const ws = toKanbanTask(wsPayload(pendingAction));

    expect(http).toEqual(ws);
    expect((http as Record<string, unknown>).primarySessionPendingAction).toBe("clarification");
  });

  it("drops unrecognized primary session pending action values", () => {
    const invalidPendingAction = {
      primary_session_pending_action: "unknown",
    } as Record<string, unknown> as Partial<TaskLike>;

    expect(toKanbanTask(httpDTO(invalidPendingAction)).primarySessionPendingAction).toBeUndefined();
    expect(
      toKanbanTask(wsPayload(invalidPendingAction)).primarySessionPendingAction,
    ).toBeUndefined();
  });

  it("maps valid task-wide pending actions from HTTP and WS shapes", () => {
    const pendingAction = { task_pending_action: "permission" } as Partial<TaskLike>;
    expect(toKanbanTask(httpDTO(pendingAction)).taskPendingAction).toBe("permission");
    expect(toKanbanTask(wsPayload(pendingAction)).taskPendingAction).toBe("permission");
  });

  it("drops unrecognized task-wide pending action values", () => {
    const invalid = {
      task_pending_action: "unknown",
    } as Record<string, unknown> as Partial<TaskLike>;
    expect(toKanbanTask(httpDTO(invalid)).taskPendingAction).toBeUndefined();
    expect(toKanbanTask(wsPayload(invalid)).taskPendingAction).toBeUndefined();
  });

  it("maps the interrupted marker from HTTP and WS shapes", () => {
    const marked = { interrupted: true } as Partial<TaskLike>;
    const http = toKanbanTask(httpDTO(marked));
    const ws = toKanbanTask(wsPayload(marked));
    expect(http.interrupted).toBe(true);
    expect(ws.interrupted).toBe(true);
    // Absent field maps to undefined so a partial update can never synthesize
    // an interrupted reading.
    expect(toKanbanTask(httpDTO()).interrupted).toBeUndefined();
    expect(toKanbanTask(wsPayload()).interrupted).toBeUndefined();
  });

  it("maps the bounded status summary from HTTP and WS task shapes", () => {
    const statusSummary = {
      revision: 7,
      updated_at: "2026-08-01T18:56:13.512Z",
      pending_action: "permission" as const,
      git: { additions: 4, deletions: 2, changed_files: 3 },
      pull_request: { count: 1, open_count: 1, attention: true, number: 42 },
    };
    const http = toKanbanTask(httpDTO({ status_summary: statusSummary }));
    const ws = toKanbanTask(wsPayload({ status_summary: statusSummary }));

    expect(http.statusSummary).toEqual(statusSummary);
    expect(ws.statusSummary).toEqual(statusSummary);
    expect(http).toEqual(ws);
  });
});

describe("toKanbanTask — autopilot", () => {
  it("preserves the immutable task creation mode for HTTP and websocket payloads", () => {
    expect(toKanbanTask(httpDTO({ autopilot: true }))).toMatchObject({ autopilot: true });
    expect(toKanbanTask(wsPayload({ autopilot: true }))).toMatchObject({ autopilot: true });
    expect(toKanbanTask(httpDTO()).autopilot).toBeUndefined();
  });
});

describe("toKanbanTask — state normalization", () => {
  it("maps the task-wide active subagent count for future status surfaces", () => {
    const mapped = toKanbanTask(
      wsPayload({ active_subagent_count: 3 } as Partial<TaskLike>),
    ) as ReturnType<typeof toKanbanTask> & { activeSubagentCount?: number };
    expect(mapped.activeSubagentCount).toBe(3);
  });

  it("preserves explicit null pending and activity fields so snapshot updates clear stale values", () => {
    const mapped = toKanbanTask(
      wsPayload({
        primary_session_pending_action: null,
        task_pending_action: null,
        foreground_activity: null,
      }),
    );

    expect(mapped.primarySessionPendingAction).toBeNull();
    expect(mapped.taskPendingAction).toBeNull();
    expect(mapped.foregroundActivity).toBeNull();
  });

  it("missing repository on either shape: repositoryId is undefined", () => {
    const http = httpDTO({ repositories: undefined });
    const ws = wsPayload({ repository_id: undefined, repositories: undefined });
    expect(toKanbanTask(http).repositoryId).toBeUndefined();
    expect(toKanbanTask(ws).repositoryId).toBeUndefined();
  });

  it("null/empty/omitted metadata yields false flags and no issue fields", () => {
    // WS omits the field entirely when Task.Metadata is nil; HTTP may send
    // null/{}. All three must derive the same derived flags.
    const cases: TaskLike[] = [
      httpDTO({ metadata: null }),
      wsPayload({ metadata: undefined }),
      wsPayload({ metadata: {} }),
    ];
    const mapped = cases.map(toKanbanTask);
    const first = mapped[0];
    expect(first.isPRReview).toBe(false);
    expect(first.isIssueWatch).toBe(false);
    expect(first.issueUrl).toBeUndefined();
    expect(first.issueNumber).toBeUndefined();
    expect(mapped[1].metadata).toBeUndefined();
    expect(mapped[2].metadata).toEqual({});
    const { metadata: _firstMetadata, ...firstWithoutMetadata } = first;
    for (const out of mapped.slice(1)) {
      const { metadata: _metadata, ...outWithoutMetadata } = out;
      expect(outWithoutMetadata).toEqual(firstWithoutMetadata);
    }
  });

  it("picks id from `id` (HTTP) or `task_id` (WS)", () => {
    expect(toKanbanTask(httpDTO({ id: "x", task_id: undefined })).id).toBe("x");
    expect(toKanbanTask(wsPayload({ id: undefined, task_id: "y" })).id).toBe("y");
  });

  it("defaults isRemoteExecutor to false when missing", () => {
    const http = httpDTO({ is_remote_executor: undefined });
    const ws = wsPayload({ is_remote_executor: undefined });
    expect(toKanbanTask(http).isRemoteExecutor).toBe(false);
    expect(toKanbanTask(ws).isRemoteExecutor).toBe(false);
  });

  it("clears the parent and retains shared workspace mode from a detach update", () => {
    const detached = toKanbanTask(
      wsPayload({
        parent_id: null,
        metadata: { workspace: { mode: "shared_group" } },
      }),
    );

    expect(detached.parentTaskId).toBeUndefined();
    expect(detached.workspaceMode).toBe("shared_group");
  });
});

describe("toKanbanTask priority", () => {
  it("preserves the canonical priority from HTTP and WebSocket payloads", () => {
    expect(toKanbanTask(httpDTO()).priority).toBe("critical");
    expect(toKanbanTask(wsPayload()).priority).toBe("critical");
  });
});

describe("preserveOmittedExecutorFields", () => {
  const existing = toKanbanTask(
    httpDTO({
      primary_executor_id: "exec-1",
      primary_executor_type: "worktree",
      primary_executor_name: "Worktree",
      is_remote_executor: false,
    }),
  );

  it("backfills all four executor fields when the incoming task omits them", () => {
    const incoming = toKanbanTask(
      httpDTO({
        primary_executor_id: undefined,
        primary_executor_type: undefined,
        primary_executor_name: undefined,
        is_remote_executor: undefined,
      }),
    );

    preserveOmittedExecutorFields(incoming, existing);

    expect(incoming.primaryExecutorId).toBe("exec-1");
    expect(incoming.primaryExecutorType).toBe("worktree");
    expect(incoming.primaryExecutorName).toBe("Worktree");
    expect(incoming.isRemoteExecutor).toBe(false);
  });

  it("leaves a real incoming executor value untouched", () => {
    const incoming = toKanbanTask(
      httpDTO({
        primary_executor_id: "exec-2",
        primary_executor_type: "ssh",
        primary_executor_name: "Remote box",
        is_remote_executor: true,
      }),
    );

    preserveOmittedExecutorFields(incoming, existing);

    expect(incoming.primaryExecutorId).toBe("exec-2");
    expect(incoming.primaryExecutorType).toBe("ssh");
    expect(incoming.primaryExecutorName).toBe("Remote box");
    expect(incoming.isRemoteExecutor).toBe(true);
  });
});
