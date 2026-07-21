import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const apiMocks = vi.hoisted(() => ({
  projects: vi.fn(),
  repositories: vi.fn(),
}));

vi.mock("@/lib/api/domains/azure-devops-api", () => ({
  listAzureDevOpsProjects: apiMocks.projects,
  listAzureDevOpsRepositories: apiMocks.repositories,
}));

import { useAzureDevOpsProjects } from "./use-azure-devops-projects";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((next) => {
    resolve = next;
  });
  return { promise, resolve };
}

beforeEach(() => vi.clearAllMocks());
afterEach(cleanup);

describe("useAzureDevOpsProjects", () => {
  it("clears loading when the list becomes inactive", async () => {
    const pending = deferred<{ projects: never[] }>();
    apiMocks.projects.mockReturnValue(pending.promise);
    const { result, rerender } = renderHook(
      ({ active }) => useAzureDevOpsProjects("workspace-a", active),
      { initialProps: { active: true } },
    );
    await waitFor(() => expect(result.current.loading).toBe(true));

    rerender({ active: false });
    expect(result.current.loading).toBe(false);
    expect(result.current.data).toEqual([]);
  });

  it("ignores a stale response after the workspace changes", async () => {
    const stale = deferred<{ projects: Array<{ id: string; name: string; url: string }> }>();
    apiMocks.projects
      .mockReturnValueOnce(stale.promise)
      .mockResolvedValueOnce({ projects: [{ id: "new", name: "New", url: "new" }] });
    const { result, rerender } = renderHook(
      ({ workspaceId }) => useAzureDevOpsProjects(workspaceId),
      { initialProps: { workspaceId: "workspace-a" } },
    );
    rerender({ workspaceId: "workspace-b" });
    await waitFor(() => expect(result.current.data[0]?.id).toBe("new"));
    await act(async () => stale.resolve({ projects: [{ id: "old", name: "Old", url: "old" }] }));
    expect(result.current.data[0]?.id).toBe("new");
  });

  it("reloads projects on refresh", async () => {
    apiMocks.projects.mockResolvedValue({ projects: [] });
    const { result } = renderHook(() => useAzureDevOpsProjects("workspace-a"));
    await waitFor(() => expect(apiMocks.projects).toHaveBeenCalledTimes(1));
    act(() => result.current.refresh());
    await waitFor(() => expect(apiMocks.projects).toHaveBeenCalledTimes(2));
  });
});
