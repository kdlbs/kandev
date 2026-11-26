import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import { createAppStore } from "@/lib/state/store";
import { querySidebarTasks } from "@/lib/api/domains/kanban-api";
import type { SidebarTaskPageResponse, SidebarTaskQuery } from "@/lib/types/http";
import { SidebarTaskPageCache } from "./sidebar-task-page-cache";

vi.mock("@/lib/api/domains/kanban-api", () => ({ querySidebarTasks: vi.fn() }));

const query: SidebarTaskQuery = {
  filters: [],
  collapsed_group_keys: [],
  collapsed_task_ids: [],
  sort: { key: "state", direction: "asc" },
  group: "none",
  page: 1,
  page_size: 100,
  locale: "en",
};
const recoveredQueryKey = "recovered";
const busy = () => new ApiError("busy", 503, { code: "persistence_unavailable" });
const page = (key: string): SidebarTaskPageResponse => ({
  query_key: key,
  page: 1,
  page_size: 100,
  total_entries: 0,
  total_tasks: 0,
  total_visible_tasks: 0,
  has_previous: false,
  has_next: false,
  entries: [],
});

function setup() {
  const store = createAppStore();
  store.setState({ workspaces: { items: [], activeId: "ws" }, workspaceContextGeneration: 1 });
  return new SidebarTaskPageCache(store);
}

beforeEach(() => {
  vi.resetAllMocks();
  vi.useFakeTimers();
});

afterEach(() => vi.useRealTimers());

describe("cold sidebar page recovery cooldown", () => {
  it("re-admits background recovery after cooldown when no rows were retained", async () => {
    const cache = setup();
    vi.mocked(querySidebarTasks).mockRejectedValue(busy());
    const initial = cache.request("ws", query, "view");
    const initialError = initial.promise.catch((error: unknown) => error);

    await vi.advanceTimersByTimeAsync(27_000);
    await expect(initialError).resolves.toMatchObject({ status: 503 });
    expect(querySidebarTasks).toHaveBeenCalledTimes(5);
    const exhaustedAt = Date.now();

    await vi.advanceTimersByTimeAsync(59_999);
    expect(Date.now() - exhaustedAt).toBe(59_999);
    const stillCooling = cache.request("ws", query, "view");
    await expect(stillCooling.promise).rejects.toMatchObject({ status: 503 });
    expect(querySidebarTasks).toHaveBeenCalledTimes(5);

    await vi.advanceTimersByTimeAsync(1);
    vi.mocked(querySidebarTasks).mockResolvedValueOnce(page(recoveredQueryKey));
    const resumed = cache.request("ws", query, "view");
    const resumedResult = resumed.promise.then(
      (response) => ({ response }),
      (error: unknown) => ({ error }),
    );
    await vi.advanceTimersByTimeAsync(0);
    await expect(resumedResult).resolves.toEqual({
      response: expect.objectContaining({ query_key: recoveredQueryKey }),
    });
    expect(querySidebarTasks).toHaveBeenCalledTimes(6);

    initial.release();
    stillCooling.release();
    resumed.release();
  });
});
