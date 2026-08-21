import { describe, expect, it } from "vitest";
import { collectTaskStatusSummaryCandidates } from "./use-task-status-summary";

const summary = (revision: number) => ({
  revision,
  updated_at: `2026-08-20T00:00:0${revision}Z`,
});

describe("collectTaskStatusSummaryCandidates", () => {
  it("collects the task from live kanban, snapshots, and archived lists", () => {
    const liveSummary = summary(4);
    const candidates = collectTaskStatusSummaryCandidates(
      {
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
