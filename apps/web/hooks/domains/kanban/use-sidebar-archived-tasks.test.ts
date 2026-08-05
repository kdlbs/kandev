import { beforeEach, describe, expect, it, vi } from "vitest";
import { listTasksByWorkspace } from "@/lib/api/domains/kanban-api";
import { loadSidebarArchivedTasks } from "./use-sidebar-archived-tasks";

vi.mock("@/lib/api/domains/kanban-api", () => ({
  listTasksByWorkspace: vi.fn(),
}));

describe("loadSidebarArchivedTasks", () => {
  beforeEach(() => vi.mocked(listTasksByWorkspace).mockReset());

  it("loads every archived-only page and stops at the reported total", async () => {
    vi.mocked(listTasksByWorkspace)
      .mockResolvedValueOnce({
        tasks: [{ id: "archived-1" }],
        total: 2,
      } as never)
      .mockResolvedValueOnce({
        tasks: [{ id: "archived-2" }],
        total: 2,
      } as never);

    await expect(loadSidebarArchivedTasks("ws-1")).resolves.toEqual([
      { id: "archived-1" },
      { id: "archived-2" },
    ]);
    expect(listTasksByWorkspace).toHaveBeenNthCalledWith(1, "ws-1", {
      page: 1,
      pageSize: 100,
      onlyArchived: true,
    });
    expect(listTasksByWorkspace).toHaveBeenNthCalledWith(2, "ws-1", {
      page: 2,
      pageSize: 100,
      onlyArchived: true,
    });
  });
});
