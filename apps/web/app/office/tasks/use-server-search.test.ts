import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { searchTasks } from "@/lib/api/domains/office-api";
import type { OfficeTask } from "@/lib/state/slices/office/types";
import { useServerSearch } from "./use-server-search";

vi.mock("@/lib/api/domains/office-api", () => ({
  searchTasks: vi.fn(),
}));

const mockSearchTasks = vi.mocked(searchTasks);

const searchedTask: OfficeTask = {
  id: "task-1",
  workspaceId: "workspace-1",
  identifier: "TASK-1",
  title: "Searched task",
  status: "todo",
  priority: "none",
  createdAt: "2026-01-01T00:00:00.000Z",
  updatedAt: "2026-01-01T00:00:00.000Z",
};

describe("useServerSearch", () => {
  beforeEach(() => {
    mockSearchTasks.mockReset();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("patches the active result when a searched card changes status", async () => {
    mockSearchTasks.mockResolvedValue({ tasks: [searchedTask] });
    const { result } = renderHook(() => useServerSearch("workspace-1"));

    await act(async () => {
      result.current.triggerSearch("searched");
      await new Promise((resolve) => setTimeout(resolve, 350));
    });
    await waitFor(() => expect(result.current.searchResults).toEqual([searchedTask]));

    act(() => result.current.patchSearchResult("task-1", { status: "in_progress" }));

    expect(result.current.searchResults).toEqual([
      expect.objectContaining({ id: "task-1", status: "in_progress" }),
    ]);
  });
});
