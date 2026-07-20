import { afterEach, describe, expect, it, vi } from "vitest";
import { createElement, type ReactNode } from "react";
import { act, renderHook, cleanup } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { prKey, usePRKeyToTasks } from "./use-pr-key-to-tasks";
import type { TaskPR } from "@/lib/types/github";

const workspacePRs = vi.hoisted(() => ({
  value: {} as Record<string, TaskPR[] | unknown>,
}));

vi.mock("./use-task-pr", () => ({
  useWorkspacePRs: () => workspacePRs.value,
}));

afterEach(() => {
  cleanup();
  workspacePRs.value = {};
});

function makeTaskPR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "pr",
    task_id: "task-1",
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

function wrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, null, children);
}

function renderUsePRKeyToTasks() {
  return renderHook(() => usePRKeyToTasks("ws-1"), { wrapper });
}

describe("prKey", () => {
  it("formats owner/repo#number", () => {
    expect(prKey("kdlbs", "kandev", 42)).toBe("kdlbs/kandev#42");
  });

  it("handles underscores and dashes in owner/repo", () => {
    expect(prKey("my-org", "my_repo", 1)).toBe("my-org/my_repo#1");
  });

  it("handles large PR numbers", () => {
    expect(prKey("foo", "bar", 99999)).toBe("foo/bar#99999");
  });

  it("handles zero PR number", () => {
    expect(prKey("o", "r", 0)).toBe("o/r#0");
  });

  it("handles single-character owner and repo", () => {
    expect(prKey("a", "b", 7)).toBe("a/b#7");
  });
});

describe("usePRKeyToTasks", () => {
  it("returns an empty map when no task PRs are loaded", () => {
    const { result } = renderUsePRKeyToTasks();
    expect(result.current.size).toBe(0);
  });

  it("groups distinct tasks linked to the same PR under one key", () => {
    const { result, rerender } = renderUsePRKeyToTasks();
    act(() => {
      workspacePRs.value = {
        "task-a": [
          makeTaskPR({ id: "row-1", task_id: "task-a", owner: "o", repo: "r", pr_number: 7 }),
        ],
        "task-b": [
          makeTaskPR({ id: "row-2", task_id: "task-b", owner: "o", repo: "r", pr_number: 7 }),
        ],
      };
      rerender();
    });
    const entries = result.current.get("o/r#7");
    expect(entries?.length).toBe(2);
    expect(entries?.map((e) => e.task_id).sort()).toEqual(["task-a", "task-b"]);
  });

  it("keeps PRs that belong to different keys separate", () => {
    const { result, rerender } = renderUsePRKeyToTasks();
    act(() => {
      workspacePRs.value = {
        "task-a": [
          makeTaskPR({ id: "row-1", task_id: "task-a", owner: "o", repo: "r", pr_number: 1 }),
          makeTaskPR({ id: "row-2", task_id: "task-a", owner: "o", repo: "r", pr_number: 2 }),
        ],
      };
      rerender();
    });
    expect(result.current.get("o/r#1")?.length).toBe(1);
    expect(result.current.get("o/r#2")?.length).toBe(1);
  });

  it("skips entries whose value is not an array (defensive against partial hydration)", () => {
    const { result, rerender } = renderUsePRKeyToTasks();
    act(() => {
      workspacePRs.value = {
        "task-a": [
          makeTaskPR({ id: "row-1", task_id: "task-a", owner: "o", repo: "r", pr_number: 1 }),
        ],
        // Partial hydration may briefly seed byTaskId[task] with a non-array
        // (e.g. an empty object). The hook should ignore those rows instead of
        // throwing.
        "task-bad": {},
      };
      rerender();
    });
    expect(result.current.get("o/r#1")?.length).toBe(1);
    expect(result.current.size).toBe(1);
  });
});
