import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createElement, type ReactNode } from "react";
import { act, cleanup, renderHook } from "@testing-library/react";
import { StateProvider, useAppStore } from "@/components/state-provider";
import type { TaskPR } from "@/lib/types/github";
import {
  findReviewPRByKey,
  resolveReviewPRSelection,
  useReviewPRSelection,
} from "./use-review-pr-selection";

const requestMock = vi.hoisted(() => vi.fn());
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: requestMock }),
}));

afterEach(() => cleanup());
beforeEach(() => {
  requestMock.mockReset().mockResolvedValue({ prs: [] });
});

function wrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, null, children);
}

function makeTaskPR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "pr-1",
    task_id: "task-1",
    owner: "acme",
    repo: "widget",
    pr_number: 1,
    pr_url: "https://github.com/acme/widget/pull/1",
    pr_title: "First branch",
    head_branch: "feat/first",
    base_branch: "main",
    author_login: "alice",
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
    created_at: "2026-07-01T00:00:00Z",
    merged_at: null,
    closed_at: null,
    last_synced_at: null,
    updated_at: "2026-07-01T00:00:00Z",
    ...overrides,
  };
}

describe("resolveReviewPRSelection", () => {
  it("defaults to the primary PR when no override exists", () => {
    const primary = makeTaskPR();

    expect(resolveReviewPRSelection([primary])).toEqual(primary);
  });

  it("selects an explicit sibling PR instead of always returning the primary", () => {
    const primary = makeTaskPR();
    const sibling = makeTaskPR({
      id: "pr-2",
      pr_number: 2,
      pr_url: "https://github.com/acme/widget/pull/2",
      pr_title: "Second branch",
      head_branch: "feat/second",
    });

    expect(resolveReviewPRSelection([primary, sibling], "acme/widget/2")).toEqual(sibling);
  });

  it("falls back to the primary PR when the selected sibling disappears", () => {
    const primary = makeTaskPR();

    expect(resolveReviewPRSelection([primary], "acme/widget/2")).toEqual(primary);
  });
});

describe("findReviewPRByKey", () => {
  it("does not substitute the primary PR when an exact panel PR disappears", () => {
    const primary = makeTaskPR();

    expect(findReviewPRByKey([primary], "acme/widget/2")).toBeNull();
  });
});

describe("useReviewPRSelection", () => {
  it("reads task PRs without starting another task-level sync loop", () => {
    renderHook(() => useReviewPRSelection("task-1"), { wrapper });

    expect(requestMock).not.toHaveBeenCalled();
  });

  it("updates the selected PR for the current task", () => {
    const primary = makeTaskPR();
    const sibling = makeTaskPR({ id: "pr-2", pr_number: 2, head_branch: "feat/second" });
    const { result } = renderHook(
      () => {
        const setTaskPRs = useAppStore((state) => state.setTaskPRs);
        return {
          setTaskPRs,
          selection: useReviewPRSelection("task-1"),
        };
      },
      { wrapper },
    );

    act(() => result.current.setTaskPRs({ "task-1": [primary, sibling] }));
    act(() => result.current.selection.selectPR(sibling));

    expect(result.current.selection.selectedPR).toEqual(sibling);
  });
});
