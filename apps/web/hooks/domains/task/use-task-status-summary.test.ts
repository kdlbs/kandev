import { describe, expect, it } from "vitest";
import { collectTaskStatusSummaryCandidates } from "./use-task-status-summary";
import type { TaskStatusSummary } from "@/lib/types/task-status-summary";

const summary = (revision: number) => ({
  revision,
  updated_at: `2026-08-20T00:00:0${revision}Z`,
});

describe("collectTaskStatusSummaryCandidates", () => {
  it("finds a queued task held only in the shared task store", () => {
    const queuedSummary: TaskStatusSummary = {
      ...summary(5),
      launch_queue: {
        reason: "session_capacity",
        queued_at: "2026-08-20T00:00:05Z",
        retrying: true,
      },
    };
    const state = {
      taskOverview: {
        byId: {
          "task-1": { id: "task-1", statusSummary: queuedSummary },
          other: { id: "other", statusSummary: summary(6) },
        },
      },
      kanban: { tasks: [] },
      kanbanMulti: { snapshots: {} },
      sidebarArchivedTasks: { itemsByWorkspaceId: {} },
    };

    expect(collectTaskStatusSummaryCandidates(state, "task-1")).toEqual([queuedSummary]);
  });

  it("collects the task from live kanban, snapshots, and archived lists", () => {
    const liveSummary = summary(4);
    const candidates = collectTaskStatusSummaryCandidates(
      {
        taskOverview: { byId: {} },
        kanban: {
          tasks: [{ id: "other", statusSummary: summary(1) }],
        },
        kanbanMulti: {
          snapshots: {
            workflow: { tasks: [{ id: "task-1", statusSummary: liveSummary }] },
          },
        },
        sidebarArchivedTasks: {
          itemsByWorkspaceId: {
            workspace: [{ id: "task-1", statusSummary: summary(3) }],
          },
        },
      },
      "task-1",
    );

    expect(candidates).toEqual([liveSummary, expect.objectContaining({ revision: 3 })]);
  });
});
