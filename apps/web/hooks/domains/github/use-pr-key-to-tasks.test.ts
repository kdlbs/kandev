import { afterEach, describe, expect, it, vi } from "vitest";
import { createElement, type ReactNode } from "react";
import { act, renderHook, cleanup } from "@testing-library/react";
import { StateProvider, useAppStore } from "@/components/state-provider";
import { prKey, usePRKeyToTasks } from "./use-pr-key-to-tasks";
import type { TaskPR } from "@/lib/types/github";

vi.mock("./use-task-pr", () => ({
  // The hook is fire-and-forget side-effect; mocking it keeps the test
  // focused on the inversion logic and avoids a real network call.
  useWorkspacePRs: () => undefined,
}));

afterEach(() => cleanup());

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

// Render usePRKeyToTasks alongside the store's setTaskPRs action so tests can
// seed the store and re-read the inverted map within the same render tree.
function renderUsePRKeyToTasks() {
  return renderHook(
    () => {
      const setTaskPRs = useAppStore((s) => s.setTaskPRs);
      const map = usePRKeyToTasks("ws-1");
      return { setTaskPRs, map };
    },
    { wrapper },
  );
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
    expect(result.current.map.size).toBe(0);
  });

  it("groups distinct tasks linked to the same PR under one key", () => {
    const { result } = renderUsePRKeyToTasks();
    act(() => {
      result.current.setTaskPRs({
        "task-a": [
          makeTaskPR({ id: "row-1", task_id: "task-a", owner: "o", repo: "r", pr_number: 7 }),
        ],
        "task-b": [
          makeTaskPR({ id: "row-2", task_id: "task-b", owner: "o", repo: "r", pr_number: 7 }),
        ],
      });
    });
    const entries = result.current.map.get("o/r#7");
    expect(entries?.length).toBe(2);
    expect(entries?.map((e) => e.task_id).sort()).toEqual(["task-a", "task-b"]);
  });

  it("keeps PRs that belong to different keys separate", () => {
    const { result } = renderUsePRKeyToTasks();
    act(() => {
      result.current.setTaskPRs({
        "task-a": [
          makeTaskPR({ id: "row-1", task_id: "task-a", owner: "o", repo: "r", pr_number: 1 }),
          makeTaskPR({ id: "row-2", task_id: "task-a", owner: "o", repo: "r", pr_number: 2 }),
        ],
      });
    });
    expect(result.current.map.get("o/r#1")?.length).toBe(1);
    expect(result.current.map.get("o/r#2")?.length).toBe(1);
  });

  it("skips entries whose value is not an array (defensive against partial hydration)", () => {
    const { result } = renderUsePRKeyToTasks();
    act(() => {
      result.current.setTaskPRs({
        "task-a": [
          makeTaskPR({ id: "row-1", task_id: "task-a", owner: "o", repo: "r", pr_number: 1 }),
        ],
        // Partial hydration may briefly seed byTaskId[task] with a non-array
        // (e.g. an empty object). The hook should ignore those rows instead of
        // throwing.
        "task-bad": {} as unknown as TaskPR[],
      });
    });
    expect(result.current.map.get("o/r#1")?.length).toBe(1);
    expect(result.current.map.size).toBe(1);
  });
});
