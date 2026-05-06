"use client";

import { useRouter } from "next/navigation";
import { ScrollArea } from "@kandev/ui/scroll-area";
import { useAppStore } from "@/components/state-provider";
import type { OfficeTask, OfficeTaskStatus } from "@/lib/state/slices/office/types";
import { StatusIcon } from "./status-icon";

const FALLBACK_COLUMNS: { status: OfficeTaskStatus; label: string }[] = [
  { status: "backlog", label: "Backlog" },
  { status: "todo", label: "Todo" },
  { status: "in_progress", label: "In Progress" },
  { status: "in_review", label: "In Review" },
  { status: "blocked", label: "Blocked" },
  { status: "done", label: "Done" },
  { status: "cancelled", label: "Cancelled" },
];

type TaskBoardProps = {
  tasks: OfficeTask[];
};

function BoardCard({ task }: { task: OfficeTask }) {
  const router = useRouter();

  return (
    <div
      className="rounded-md border border-border bg-card p-3 hover:bg-accent/50 transition-colors cursor-pointer"
      onClick={() => router.push(`/office/tasks/${task.id}`)}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => e.key === "Enter" && router.push(`/office/tasks/${task.id}`)}
    >
      <div className="flex items-center gap-1.5 mb-1">
        <StatusIcon status={task.status} className="h-3.5 w-3.5 shrink-0" />
        <span className="text-[11px] text-muted-foreground font-mono">{task.identifier}</span>
      </div>
      <p className="text-sm truncate">{task.title}</p>
    </div>
  );
}

function BoardColumn({
  label,
  status,
  tasks,
}: {
  label: string;
  status: OfficeTaskStatus;
  tasks: OfficeTask[];
}) {
  return (
    <div className="flex flex-col min-w-[240px] max-w-[300px] flex-1">
      <div className="flex items-center gap-2 px-2 py-2 mb-2">
        <StatusIcon status={status} className="h-3.5 w-3.5" />
        <span className="text-xs font-medium">{label}</span>
        <span className="text-xs text-muted-foreground">{tasks.length}</span>
      </div>
      <ScrollArea className="flex-1">
        <div className="flex flex-col gap-1.5 px-1 pb-2">
          {tasks.map((task) => (
            <BoardCard key={task.id} task={task} />
          ))}
        </div>
      </ScrollArea>
    </div>
  );
}

export function TaskBoard({ tasks }: TaskBoardProps) {
  const meta = useAppStore((s) => s.office.meta);
  const columns = meta
    ? meta.statuses.map((s) => ({ status: s.id as OfficeTaskStatus, label: s.label }))
    : FALLBACK_COLUMNS;

  const grouped = new Map<OfficeTaskStatus, OfficeTask[]>();
  for (const col of columns) {
    grouped.set(col.status, []);
  }
  for (const task of tasks) {
    const list = grouped.get(task.status);
    if (list) list.push(task);
  }

  return (
    <div className="flex gap-3 overflow-x-auto pb-4">
      {columns.map((col) => (
        <BoardColumn
          key={col.status}
          label={col.label}
          status={col.status}
          tasks={grouped.get(col.status) ?? []}
        />
      ))}
    </div>
  );
}
