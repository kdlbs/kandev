/* eslint-disable sonarjs/no-duplicate-string */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStore } from "@/components/state-provider";
import type { OfficeTask } from "@/lib/state/slices/office/types";
import { listTasks as listOfficeTasks } from "@/lib/api/domains/office-tasks-api";
import { listTasks as listOfficeTasksLegacy } from "@/lib/api/domains/office-extended-api";
import { usePaginatedTasks } from "./use-paginated-tasks";

vi.mock("sonner", () => ({
  toast: { error: vi.fn() },
}));

vi.mock("@/lib/api/domains/office-tasks-api", () => ({
  listTasks: vi.fn(),
}));

vi.mock("@/lib/api/domains/office-extended-api", () => ({
  listTasks: vi.fn(),
}));

const listTasksMock = vi.mocked(listOfficeTasks);
const legacyListTasksMock = vi.mocked(listOfficeTasksLegacy);

function task(id: string, title = id): OfficeTask {
  return {
    id,
    workspaceId: "workspace-1",
    identifier: id.toUpperCase(),
    title,
    status: "todo",
    priority: "medium",
    createdAt: "2026-06-23T00:00:00Z",
    updatedAt: "2026-06-23T00:00:00Z",
  };
}

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
}

function wrapperFor(queryClient: QueryClient) {
  return function Wrapper({ children }: PropsWithChildren) {
    return (
      <QueryClientProvider client={queryClient}>
        <StateProvider>{children}</StateProvider>
      </QueryClientProvider>
    );
  };
}

function renderPaginatedTasks(queryClient: QueryClient) {
  return renderHook(
    () => ({
      pagination: usePaginatedTasks("workspace-1", false),
      tasks: useAppStore((state) => state.office.tasks.items),
      isLoading: useAppStore((state) => state.office.tasks.isLoading),
    }),
    { wrapper: wrapperFor(queryClient) },
  );
}

describe("usePaginatedTasks", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("mirrors infinite query pages into the legacy task list store", async () => {
    listTasksMock
      .mockResolvedValueOnce({
        tasks: [task("task-1")],
        next_cursor: "cursor-2",
        next_id: "task-1",
      })
      .mockResolvedValueOnce({ tasks: [task("task-2")], next_cursor: "", next_id: "" });
    legacyListTasksMock.mockImplementation(listTasksMock);
    const queryClient = createQueryClient();
    const result = renderPaginatedTasks(queryClient);

    await waitFor(() => expect(result.result.current.tasks.map((t) => t.id)).toEqual(["task-1"]));

    const cacheEntries = queryClient.getQueryCache().findAll({
      queryKey: ["office", "workspaces", "workspace-1", "tasks"],
    });
    expect(cacheEntries).toHaveLength(1);
    expect(result.result.current.pagination.hasMore).toBe(true);

    await act(async () => {
      result.result.current.pagination.loadMore();
    });

    await waitFor(() =>
      expect(result.result.current.tasks.map((t) => t.id)).toEqual(["task-1", "task-2"]),
    );
    expect(listTasksMock).toHaveBeenLastCalledWith(
      "workspace-1",
      expect.objectContaining({ cursor: "cursor-2", cursor_id: "task-1" }),
      expect.anything(),
    );
    expect(result.result.current.pagination.hasMore).toBe(false);
    expect(result.result.current.isLoading).toBe(false);
  });
});
