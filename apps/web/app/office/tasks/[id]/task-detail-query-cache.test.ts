import { QueryClient } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";
import { qk } from "@/lib/query/keys";
import type { OfficeTask } from "@/lib/state/slices/office/types";
import { readOfficeTaskFromCachedPages } from "./task-detail-query-cache";

const TIMESTAMP = "2026-07-01T00:00:00Z";
const WORKSPACE_ID = "workspace-1";
const CACHED_TASK_ID = "task-2";

function task(id: string, overrides: Partial<OfficeTask> = {}): OfficeTask {
  return {
    id,
    workspaceId: WORKSPACE_ID,
    identifier: id.toUpperCase(),
    title: id,
    status: "todo",
    priority: "medium",
    createdAt: TIMESTAMP,
    updatedAt: TIMESTAMP,
    ...overrides,
  };
}

describe("task detail query cache helpers", () => {
  it("reads an office task from cached infinite task pages", () => {
    const queryClient = new QueryClient();
    const cachedTask = task(CACHED_TASK_ID, { title: "Cached task" });
    queryClient.setQueryData(qk.office.tasks(WORKSPACE_ID, { limit: 200 }), {
      pages: [{ tasks: [task("task-1"), cachedTask] }],
      pageParams: [undefined],
    });

    expect(readOfficeTaskFromCachedPages(queryClient, WORKSPACE_ID, CACHED_TASK_ID)).toBe(
      cachedTask,
    );
    expect(readOfficeTaskFromCachedPages(queryClient, "workspace-2", CACHED_TASK_ID)).toBeNull();
  });

  it("ignores malformed cached task pages", () => {
    const queryClient = new QueryClient();
    queryClient.setQueryData(qk.office.tasks(WORKSPACE_ID, { limit: 200 }), {
      pages: [null, { other: [] }, { tasks: [null, { id: null }, task("task-1")] }],
      pageParams: [undefined],
    });

    expect(readOfficeTaskFromCachedPages(queryClient, WORKSPACE_ID, "missing")).toBeNull();
  });
});
