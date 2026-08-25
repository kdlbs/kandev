"use client";

import { useRouter } from "@/lib/routing/client-router";
import { DndContext, DragOverlay, pointerWithin, useDraggable, useDroppable } from "@dnd-kit/core";
import { ScrollArea } from "@kandev/ui/scroll-area";
import { cn } from "@/lib/utils";
import { useAppStore } from "@/components/state-provider";
import type { OfficeTask, OfficeTaskStatus } from "@/lib/state/slices/office/types";
import { StatusIcon } from "./status-icon";
import { STATUS_LABEL_KEYS } from "../lib/label-keys";
import { useBoardDrag } from "./use-board-drag";
import { useTranslation } from "react-i18next";

// `labelKey`, not `label` — see the note in `status-labels.ts`. The workspace's
// own status metadata wins when present, and its labels are workspace data.
const FALLBACK_COLUMNS: { status: OfficeTaskStatus; labelKey: string }[] = [
  { status: "backlog", labelKey: STATUS_LABEL_KEYS.backlog },
  { status: "todo", labelKey: STATUS_LABEL_KEYS.todo },
  { status: "in_progress", labelKey: STATUS_LABEL_KEYS.in_progress },
  { status: "in_review", labelKey: STATUS_LABEL_KEYS.in_review },
  { status: "blocked", labelKey: STATUS_LABEL_KEYS.blocked },
  { status: "done", labelKey: STATUS_LABEL_KEYS.done },
  { status: "cancelled", labelKey: STATUS_LABEL_KEYS.cancelled },
];

const CARD_CLASS = "rounded-md border border-border bg-card p-3";

type TaskBoardProps = {
  tasks: OfficeTask[];
  onTaskPatch?: (taskId: string, patch: Partial<OfficeTask>) => void;
};

function CardBody({ task }: { task: OfficeTask }) {
  return (
    <>
      <div className="flex items-center gap-1.5 mb-1">
        <StatusIcon status={task.status} className="h-3.5 w-3.5 shrink-0" />
        <span className="text-[11px] text-muted-foreground font-mono">{task.identifier}</span>
      </div>
      <p className="text-sm truncate">{task.title}</p>
    </>
  );
}

function BoardCard({ task }: { task: OfficeTask }) {
  const router = useRouter();
  const { attributes, listeners, setNodeRef, isDragging } = useDraggable({ id: task.id });
  const open = () => router.push(`/office/tasks/${task.id}`);

  return (
    <div
      ref={setNodeRef}
      data-testid={`board-card-${task.id}`}
      className={cn(
        CARD_CLASS,
        "hover:bg-accent/50 transition-colors cursor-pointer",
        // The card in flight is drawn by the DragOverlay; fading the original
        // keeps it from reading as a duplicate.
        isDragging && "opacity-40",
      )}
      onClick={open}
      onKeyDown={(e) => e.key === "Enter" && open()}
      // `attributes` supplies role="button" and tabIndex, so the card stays
      // focusable and Enter still opens the task.
      {...attributes}
      {...listeners}
    >
      <CardBody task={task} />
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
  const { setNodeRef, isOver } = useDroppable({ id: status });

  return (
    // The droppable ref lives here, not on the inner list: that list sits
    // inside a Radix ScrollArea viewport whose content box is content-height
    // (display: table), while this root stretches to the row's tallest
    // column via the parent flex row's default align-items: stretch. Ref'ing
    // the inner box means a short column's droppable is only as tall as its
    // own cards, so dropping in the visible-but-blank space below them finds
    // no droppable and the card silently snaps back.
    <div
      ref={setNodeRef}
      data-testid={`board-column-${status}`}
      className={cn(
        "flex flex-col min-w-[240px] max-w-[300px] flex-1 rounded-md transition-colors",
        isOver && "bg-accent/40 ring-1 ring-primary/40",
      )}
    >
      <div className="flex items-center gap-2 px-2 py-2 mb-2">
        <StatusIcon status={status} className="h-3.5 w-3.5" />
        <span className="text-xs font-medium">{label}</span>
        <span className="text-xs text-muted-foreground">{tasks.length}</span>
      </div>
      <ScrollArea className="flex-1">
        <div
          // min-h keeps an empty column a real drop target even before the
          // root above has any sibling tall enough to stretch it.
          className="flex flex-col gap-1.5 px-1 pb-2 min-h-[64px]"
        >
          {tasks.map((task) => (
            <BoardCard key={task.id} task={task} />
          ))}
        </div>
      </ScrollArea>
    </div>
  );
}

// Board columns are keyed on the canonical lowercase OfficeTaskStatus
// vocabulary. `task.status` is normalized to that vocabulary at ingestion
// (office-slice.ts), so no per-consumer normalization is needed here.
export function groupTasksByStatus(
  tasks: OfficeTask[],
  columnStatuses: OfficeTaskStatus[],
): Map<OfficeTaskStatus, OfficeTask[]> {
  const grouped = new Map<OfficeTaskStatus, OfficeTask[]>();
  for (const status of columnStatuses) {
    grouped.set(status, []);
  }
  for (const task of tasks) {
    const list = grouped.get(task.status);
    if (list) list.push(task);
  }
  return grouped;
}

export function TaskBoard({ tasks, onTaskPatch }: TaskBoardProps) {
  const { t } = useTranslation();
  const meta = useAppStore((s) => s.office.meta);
  const { activeTaskId, sensors, handleDragStart, handleDragEnd, handleDragCancel } = useBoardDrag(
    tasks,
    onTaskPatch,
  );
  const columns = meta
    ? meta.statuses.map((s) => ({ status: s.id as OfficeTaskStatus, label: s.label }))
    : FALLBACK_COLUMNS.map((c) => ({ status: c.status, label: t(c.labelKey) }));

  const grouped = groupTasksByStatus(
    tasks,
    columns.map((c) => c.status),
  );
  const activeTask = activeTaskId ? (tasks.find((task) => task.id === activeTaskId) ?? null) : null;

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={pointerWithin}
      onDragStart={handleDragStart}
      onDragEnd={handleDragEnd}
      onDragCancel={handleDragCancel}
    >
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
      <DragOverlay dropAnimation={null}>
        {activeTask ? (
          <div className={cn(CARD_CLASS, "shadow-lg cursor-grabbing")}>
            <CardBody task={activeTask} />
          </div>
        ) : null}
      </DragOverlay>
    </DndContext>
  );
}
