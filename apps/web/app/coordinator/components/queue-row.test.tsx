import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { AttentionTask, QueueItem } from "@/lib/coordinator/attention";
import type { TaskPR } from "@/lib/types/github";
import { QueueRow } from "./queue-row";

afterEach(cleanup);

function task(overrides: Partial<AttentionTask> = {}): AttentionTask {
  return { id: "t-1", title: "Task 1", identifier: "KAN-1", ...overrides };
}

function item(overrides: Partial<QueueItem> = {}): QueueItem {
  return {
    group: "other",
    id: "t-1",
    task: task(),
    lastActivityAtMs: undefined,
    ageMs: 5 * 60_000,
    ...overrides,
  };
}

function taskPR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "pr",
    workspace_id: "ws-1",
    task_id: "t-1",
    owner: "kdlbs",
    repo: "kandev",
    pr_number: 1,
    pr_url: "",
    pr_title: "",
    head_branch: "",
    base_branch: "",
    author_login: "",
    state: "open",
    review_state: "",
    checks_state: "",
    mergeable_state: "",
    review_count: 0,
    pending_review_count: 0,
    comment_count: 0,
    unresolved_review_threads: 0,
    checks_total: 0,
    checks_passing: 0,
    additions: 0,
    deletions: 0,
    created_at: "",
    merged_at: null,
    closed_at: null,
    last_synced_at: null,
    updated_at: "",
    ...overrides,
  };
}

const NO_PRS: ReadonlyMap<string, TaskPR[]> = new Map();

describe("QueueRow", () => {
  it("shows the identifier, step and age", () => {
    render(
      <QueueRow
        item={item()}
        stepNameByTaskId={new Map([["t-1", "Build"]])}
        prsByTaskId={NO_PRS}
      />,
    );
    expect(screen.getByText("KAN-1")).not.toBeNull();
    expect(screen.getByText("Build")).not.toBeNull();
    expect(screen.getByText("5m")).not.toBeNull();
  });

  it("shows the agent state for a Working row", () => {
    render(
      <QueueRow
        item={item({
          group: "working",
          task: task({ statusSummary: { primary_session: { id: "s-1", state: "RUNNING" } } }),
        })}
        stepNameByTaskId={new Map()}
        prsByTaskId={NO_PRS}
      />,
    );
    expect(screen.getByText("Running")).not.toBeNull();
  });

  it("shows PR detail unavailable with no PR loaded for an In review row", () => {
    render(
      <QueueRow
        item={item({
          group: "in_review",
          task: task({ statusSummary: { pull_request: { state: "open" } } }),
        })}
        stepNameByTaskId={new Map()}
        prsByTaskId={NO_PRS}
      />,
    );
    expect(screen.getByText("open")).not.toBeNull();
    expect(screen.getByText("PR detail unavailable")).not.toBeNull();
  });

  it("shows PR state, unresolved threads and checks state when the PR is loaded", () => {
    const prsByTaskId = new Map([
      ["t-1", [taskPR({ state: "open", unresolved_review_threads: 2, checks_state: "pending" })]],
    ]);
    render(
      <QueueRow
        item={item({ group: "ready_to_merge" })}
        stepNameByTaskId={new Map()}
        prsByTaskId={prsByTaskId}
      />,
    );
    expect(screen.getByText("open")).not.toBeNull();
    expect(screen.getByText("pending")).not.toBeNull();
  });

  it("shows the underivable text for an unreadable Other row", () => {
    render(
      <QueueRow
        item={item({ group: "other", sessionUnreadable: true })}
        stepNameByTaskId={new Map()}
        prsByTaskId={NO_PRS}
      />,
    );
    expect(screen.getByText("position underivable: session unreadable")).not.toBeNull();
  });

  it("shows no session for an Other row with no primary session", () => {
    render(
      <QueueRow
        item={item({
          group: "other",
          task: task({ statusSummary: { primary_session: null } }),
        })}
        stepNameByTaskId={new Map()}
        prsByTaskId={NO_PRS}
      />,
    );
    expect(screen.getByText("No session")).not.toBeNull();
  });

  it("falls back to the task title with no identifier", () => {
    render(
      <QueueRow
        item={item({ task: task({ identifier: undefined }) })}
        stepNameByTaskId={new Map()}
        prsByTaskId={NO_PRS}
      />,
    );
    expect(screen.getByText("Task 1")).not.toBeNull();
  });
});
