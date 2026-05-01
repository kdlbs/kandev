import { describe, it, expect } from "vitest";
import { toKanbanTask, type TaskLike } from "./map-task";

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
  priority: 0,
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
  it("plain task: both shapes produce identical KanbanTask", () => {
    expect(toKanbanTask(httpDTO())).toEqual(toKanbanTask(wsPayload()));
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
    for (const out of mapped.slice(1)) {
      expect(out).toEqual(first);
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
});
