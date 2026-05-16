import { listTasks } from "@/lib/api/domains/office-api";
import { getActiveWorkspaceId } from "../lib/get-active-workspace";
import { TasksPageClient } from "./tasks-page-client";
import type { OfficeTask } from "@/lib/state/slices/office/types";

export default async function TasksPage() {
  const workspaceId = await getActiveWorkspaceId();

  let tasks: OfficeTask[] = [];
  if (workspaceId) {
    const res = await listTasks(workspaceId, { cache: "no-store" }).catch(() => ({
      tasks: [],
    }));
    tasks = res.tasks ?? [];
  }

  return <TasksPageClient initialIssues={tasks} />;
}
