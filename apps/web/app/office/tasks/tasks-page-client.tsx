"use client";

import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import type { OfficeTask } from "@/lib/state/slices/office/types";
import { TasksList } from "./tasks-list";

type IssuesPageClientProps = {
  initialIssues: OfficeTask[];
};

export function TasksPageClient({ initialIssues }: IssuesPageClientProps) {
  const setTasks = useAppStore((s) => s.setTasks);

  // Hydrate the store from SSR so the first paint shows tasks before the
  // client-side filtered fetch in TasksList resolves. TasksList owns the
  // ongoing fetch / filter / pagination / WS-refetch lifecycle.
  useEffect(() => {
    if (initialIssues.length > 0) setTasks(initialIssues);
  }, [initialIssues, setTasks]);

  return <TasksList />;
}
