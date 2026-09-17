import { expect, it } from "vitest";
import type { Task } from "@/lib/types/http";
import { coordinatorTaskTotals } from "./coordinator-task-totals";
it("reports known coverage separately and excludes unavailable git comparisons", () => {
  const tasks = [
    {},
    { status_summary: { pull_request: { open_count: 2 }, git: { changed_files: 3 } } },
    {
      status_summary: {
        pull_request: { open_count: 0 },
        git: { changed_files: 99, comparison_unavailable: true },
      },
    },
  ] as Task[];
  expect(coordinatorTaskTotals(tasks)).toEqual({
    tasks: 3,
    open: 2,
    prKnown: 2,
    files: 3,
    gitKnown: 1,
  });
});
