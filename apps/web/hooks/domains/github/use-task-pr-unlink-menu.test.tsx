import { act, cleanup, renderHook } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import type { AppState } from "@/lib/state/store";
import type { TaskPR } from "@/lib/types/github";

const listTaskPRsMock = vi.hoisted(() => vi.fn());
const deleteTaskPRMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/github-api", () => ({
  deleteTaskPR: deleteTaskPRMock,
  listTaskPRs: listTaskPRsMock,
}));

import { useTaskPRUnlinkMenu } from "./use-task-pr-unlink-menu";

function createWrapper() {
  return function TestWrapper({ children }: { children: ReactNode }) {
    const initialState: Partial<AppState> = {
      workspaces: { items: [], activeId: "ws-1" },
      taskPRs: {
        byTaskId: {},
        workspaceId: "ws-1",
        workspaceContextGeneration: 0,
      },
    };
    return createElement(StateProvider, {
      initialState,
      children: createElement(ToastProvider, null, children),
    });
  };
}

function makePR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "association-api-42",
    workspace_id: "ws-1",
    task_id: "task-1",
    repository_id: "repository-api",
    owner: "acme",
    repo: "api",
    pr_number: 42,
    pr_url: "https://github.com/acme/api/pull/42",
    pr_title: "API change",
    head_branch: "feature/api",
    base_branch: "main",
    author_login: "octocat",
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
    additions: 1,
    deletions: 0,
    created_at: "",
    merged_at: null,
    closed_at: null,
    last_synced_at: null,
    updated_at: "",
    ...overrides,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

beforeEach(() => {
  listTaskPRsMock.mockReset();
  deleteTaskPRMock.mockReset();
});
afterEach(() => cleanup());

describe("useTaskPRUnlinkMenu", () => {
  // @covers AC-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001.3, .6
  it("hydrates on menu open and exposes exact association identities after loading", async () => {
    const response = deferred<{ task_prs: Record<string, TaskPR[]> }>();
    listTaskPRsMock.mockReturnValue(response.promise);
    const { result } = renderHook(() => useTaskPRUnlinkMenu("task-1", 42), {
      wrapper: createWrapper(),
    });

    expect(listTaskPRsMock).not.toHaveBeenCalled();
    act(() => result.current.onOpenChange(true));

    expect(listTaskPRsMock).toHaveBeenCalledWith(["task-1"], { cache: "no-store" });
    expect(result.current.isLoading).toBe(true);
    expect(result.current.choices).toEqual([]);

    const pr = makePR();
    await act(async () => {
      response.resolve({ task_prs: { "task-1": [pr] } });
      await response.promise;
    });

    expect(result.current.isLoading).toBe(false);
    expect(result.current.choices).toEqual([
      {
        associationId: pr.id,
        owner: "acme",
        repo: "api",
        number: 42,
        pending: false,
      },
    ]);
  });

  // @covers AC-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001.6
  it("does not infer unlink identities from a summary when the scoped lookup has no rows", async () => {
    listTaskPRsMock.mockResolvedValue({ task_prs: { "task-1": [] } });
    const { result } = renderHook(() => useTaskPRUnlinkMenu("task-1", 42), {
      wrapper: createWrapper(),
    });

    act(() => result.current.onOpenChange(true));
    await act(async () => {
      await Promise.resolve();
    });

    expect(result.current.choices).toEqual([]);
    expect(result.current.isLoading).toBe(false);
    expect(deleteTaskPRMock).not.toHaveBeenCalled();
  });

  it("does not hydrate when no compact PR summary is present", () => {
    const { result } = renderHook(() => useTaskPRUnlinkMenu("task-1", null), {
      wrapper: createWrapper(),
    });

    act(() => result.current.onOpenChange(true));

    expect(listTaskPRsMock).not.toHaveBeenCalled();
    expect(result.current.isLoading).toBe(false);
  });
});
