import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";

const mockUseWorkspacePRs = vi.fn();
const mockListWorkspaceTaskPRs = vi.fn();

type MockState = {
  workspaces: { activeId: string | null };
  taskPRs: { workspaceId: string | null; byTaskId: Record<string, unknown> };
};

let mockState: MockState;

const WORKSPACE_ID = "workspace-1";

function baseState(overrides: Partial<MockState> = {}): MockState {
  return {
    workspaces: { activeId: WORKSPACE_ID },
    taskPRs: { workspaceId: null, byTaskId: {} },
    ...overrides,
  };
}

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: MockState) => unknown) => selector(mockState),
}));

vi.mock("@/hooks/domains/github/use-task-pr", () => ({
  useWorkspacePRs: (...args: unknown[]) => mockUseWorkspacePRs(...args),
}));

vi.mock("@/lib/api/domains/github-api", () => ({
  listWorkspaceTaskPRs: (...args: unknown[]) => mockListWorkspaceTaskPRs(...args),
}));

import { useCoordinatorPRs } from "./use-coordinator-prs";

beforeEach(() => {
  vi.clearAllMocks();
  mockState = baseState();
});

// The route's workspace can differ from the globally active one. `taskPRs` is
// a single non-partitioned cache (`setTaskPRs` wholesale-replaces it), so
// only the active-workspace path may fetch into it; a different workspace
// reads directly instead (mirrors use-coordinator-tasks.ts).
describe("useCoordinatorPRs - active workspace", () => {
  it("fetches via useWorkspacePRs and reads the shared taskPRs cache when it matches", () => {
    mockState = baseState({
      taskPRs: { workspaceId: WORKSPACE_ID, byTaskId: { "t-1": [{ id: "pr-1" }] } },
    });

    const { result } = renderHook(() => useCoordinatorPRs(WORKSPACE_ID));

    expect(mockUseWorkspacePRs).toHaveBeenCalledWith(WORKSPACE_ID);
    expect(result.current.get("t-1")).toEqual([{ id: "pr-1" }]);
  });

  it("returns an empty map while the cache holds a different workspace's data", () => {
    mockState = baseState({
      taskPRs: { workspaceId: "some-other-workspace", byTaskId: { "t-1": [{ id: "pr-1" }] } },
    });

    const { result } = renderHook(() => useCoordinatorPRs(WORKSPACE_ID));

    expect(result.current.size).toBe(0);
  });
});

describe("useCoordinatorPRs - non-active workspace (direct fetch)", () => {
  beforeEach(() => {
    mockState = baseState({ workspaces: { activeId: "some-other-active-workspace" } });
  });

  it("does not call useWorkspacePRs (would collide with the active workspace's cache)", () => {
    mockListWorkspaceTaskPRs.mockResolvedValue({ task_prs: {} });
    renderHook(() => useCoordinatorPRs(WORKSPACE_ID));
    expect(mockUseWorkspacePRs).toHaveBeenCalledWith(null);
  });

  it("fetches the route workspace's PRs directly", async () => {
    mockListWorkspaceTaskPRs.mockResolvedValue({ task_prs: { "t-1": [{ id: "pr-1" }] } });

    const { result } = renderHook(() => useCoordinatorPRs(WORKSPACE_ID));

    await waitFor(() => expect(result.current.get("t-1")).toEqual([{ id: "pr-1" }]));
    expect(mockListWorkspaceTaskPRs).toHaveBeenCalledWith(WORKSPACE_ID, { cache: "no-store" });
  });
});
