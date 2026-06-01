import { afterEach, describe, expect, it, vi } from "vitest";
import { createElement, type ComponentProps, type ReactNode } from "react";
import { renderHook, cleanup } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StateProvider } from "@/components/state-provider";
import { createTestQueryClient } from "@/test-utils/render-with-query";
import { qk } from "@/lib/query/keys";
import { prKey, usePRKeyToTasks } from "./use-pr-key-to-tasks";
import type { AppState } from "@/lib/state/store";
import type { TaskPR } from "@/lib/types/github";

const WS_ID = "ws-1";

vi.mock("./use-task-pr", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./use-task-pr")>();
  return {
    ...actual,
    // The fetch is fire-and-forget; mock it so the test reads only from the
    // seeded TQ cache (no real network call).
    useWorkspacePRs: () => undefined,
  };
});

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

const INITIAL_STATE = { workspaces: { activeId: WS_ID } } as Partial<AppState>;

function makeWrapper(queryClient: QueryClient) {
  return function wrapper({ children }: { children: ReactNode }) {
    return createElement(
      QueryClientProvider,
      { client: queryClient },
      createElement(
        StateProvider,
        { initialState: INITIAL_STATE } as ComponentProps<typeof StateProvider>,
        children,
      ),
    );
  };
}

// Seed the workspace PR TQ cache, then read the inverted map via the hook.
function renderUsePRKeyToTasks(taskPRs?: Record<string, unknown>) {
  const queryClient = createTestQueryClient();
  if (taskPRs) {
    queryClient.setQueryData(qk.github.prs(WS_ID), { task_prs: taskPRs });
  }
  return renderHook(() => usePRKeyToTasks(WS_ID), { wrapper: makeWrapper(queryClient) });
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
    const { result } = renderUsePRKeyToTasks({
      "task-a": [
        makeTaskPR({ id: "row-1", task_id: "task-a", owner: "o", repo: "r", pr_number: 7 }),
      ],
      "task-b": [
        makeTaskPR({ id: "row-2", task_id: "task-b", owner: "o", repo: "r", pr_number: 7 }),
      ],
    });
    const entries = result.current.get("o/r#7");
    expect(entries?.length).toBe(2);
    expect(entries?.map((e) => e.task_id).sort()).toEqual(["task-a", "task-b"]);
  });

  it("keeps PRs that belong to different keys separate", () => {
    const { result } = renderUsePRKeyToTasks({
      "task-a": [
        makeTaskPR({ id: "row-1", task_id: "task-a", owner: "o", repo: "r", pr_number: 1 }),
        makeTaskPR({ id: "row-2", task_id: "task-a", owner: "o", repo: "r", pr_number: 2 }),
      ],
    });
    expect(result.current.get("o/r#1")?.length).toBe(1);
    expect(result.current.get("o/r#2")?.length).toBe(1);
  });

  it("skips entries whose value is not an array (defensive against partial hydration)", () => {
    const { result } = renderUsePRKeyToTasks({
      "task-a": [
        makeTaskPR({ id: "row-1", task_id: "task-a", owner: "o", repo: "r", pr_number: 1 }),
      ],
      // Partial hydration may briefly seed task_prs[task] with a non-array
      // (e.g. an empty object). The hook should ignore those rows.
      "task-bad": {} as unknown as TaskPR[],
    });
    expect(result.current.get("o/r#1")?.length).toBe(1);
    expect(result.current.size).toBe(1);
  });
});
