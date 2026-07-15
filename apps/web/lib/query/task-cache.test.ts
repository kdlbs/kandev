import { QueryClient } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";
import type { WorkflowSnapshot } from "@/lib/types/http";
import { qk } from "./keys";
import { reconcileUnarchiveTaskQueries } from "./task-cache";

describe("reconcileUnarchiveTaskQueries", () => {
  it("clears archived details and invalidates task lists and workflow snapshots", () => {
    const client = new QueryClient();
    const pageKey = qk.tasks.page("workspace-1");
    const snapshotKey = qk.workflows.snapshot("workflow-1");
    client.setQueryData(qk.tasks.detail("task-1"), {
      id: "task-1",
      archived_at: "2026-07-14T12:00:00Z",
    });
    client.setQueryData(pageKey, { tasks: [], total: 0 });
    client.setQueryData(snapshotKey, {
      workflow: { id: "workflow-1", workspace_id: "workspace-1" },
      steps: [],
      tasks: [],
    } as unknown as WorkflowSnapshot);

    reconcileUnarchiveTaskQueries(client, {
      success: true,
      cascade_id: "cascade-1",
      unarchived_ids: ["task-1"],
      skipped_ids: [],
      affected_group_ids: [],
      recovery: [],
    });

    expect(client.getQueryData(qk.tasks.detail("task-1"))).toMatchObject({ archived_at: null });
    expect(client.getQueryState(pageKey)?.isInvalidated).toBe(true);
    expect(client.getQueryState(snapshotKey)?.isInvalidated).toBe(true);
  });
});
