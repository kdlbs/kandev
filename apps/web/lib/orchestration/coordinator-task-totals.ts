import type { Task } from "@/lib/types/http";

export function coordinatorTaskTotals(tasks: Task[]) {
  const totals = { tasks: tasks.length, open: 0, prKnown: 0, files: 0, gitKnown: 0 };
  for (const task of tasks) {
    const pr = task.status_summary?.pull_request;
    if (pr?.open_count !== undefined) {
      totals.open += pr.open_count;
      totals.prKnown++;
    }
    const git = task.status_summary?.git;
    if (git?.changed_files !== undefined && !git.comparison_unavailable) {
      totals.files += git.changed_files;
      totals.gitKnown++;
    }
  }
  return totals;
}
