"use client";

import { useState } from "react";
import { IconChevronDown } from "@tabler/icons-react";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { cn } from "@/lib/utils";
import { useOptimisticTaskMutation } from "@/hooks/use-optimistic-task-mutation";
import { ApiError } from "@/lib/api/client";
import { updateTask } from "@/lib/api/domains/office-extended-api";
import type { Task, TaskStatus } from "@/app/office/tasks/[id]/types";
import { StatusIcon } from "@/app/office/tasks/[id]/status-icon";
import { normalizeTaskStatus } from "@/app/office/tasks/normalize-status";

type PendingApprover = { agent_profile_id?: string; name?: string };

// formatPendingApproversMessage builds the toast text the user sees when
// the backend rejects an in_review → done transition because not every
// approver has signed off. The backend echoes a `pending_approvers`
// array on the 409 body; we render the names in order.
export function formatPendingApproversMessage(pending: PendingApprover[]): string {
  const names = pending.map((p) => p.name?.trim() || p.agent_profile_id || "").filter(Boolean);
  if (names.length === 0) return "Cannot mark done: awaiting approvals";
  return `Cannot mark done: awaiting approval from ${names.join(", ")}`;
}

function extractPendingApprovers(err: unknown): PendingApprover[] | null {
  if (!(err instanceof ApiError)) return null;
  if (err.status !== 409) return null;
  const body = err.body;
  if (!body || typeof body !== "object") return null;
  const pending = (body as { pending_approvers?: unknown }).pending_approvers;
  if (!Array.isArray(pending)) return null;
  return pending.filter((p): p is PendingApprover => !!p && typeof p === "object");
}

async function updateStatusOrTranslateGate(taskId: string, status: TaskStatus): Promise<void> {
  try {
    await updateTask(taskId, { status });
  } catch (err) {
    const pending = extractPendingApprovers(err);
    if (pending) {
      throw new Error(formatPendingApproversMessage(pending));
    }
    throw err;
  }
}

type StatusOption = { value: TaskStatus; label: string };

const STATUS_OPTIONS: StatusOption[] = [
  { value: "backlog", label: "Backlog" },
  { value: "todo", label: "Todo" },
  { value: "in_progress", label: "In Progress" },
  { value: "in_review", label: "In Review" },
  { value: "blocked", label: "Blocked" },
  { value: "done", label: "Done" },
  { value: "cancelled", label: "Cancelled" },
];

export const STATUS_LABELS: Record<TaskStatus, string> = STATUS_OPTIONS.reduce(
  (acc, opt) => {
    acc[opt.value] = opt.label;
    return acc;
  },
  {} as Record<TaskStatus, string>,
);

type StatusPickerProps = {
  task: Task;
};

export function StatusPicker({ task }: StatusPickerProps) {
  const [open, setOpen] = useState(false);
  const mutate = useOptimisticTaskMutation();
  // Backend may return the kanban-state spelling ("REVIEW", "COMPLETED",
  // …) or the canonical lowercase form ("in_review", "done"). Normalise
  // both via the shared office helper so STATUS_LABELS lookups and the
  // aria-selected comparisons agree on a single TaskStatus value.
  const current = normalizeTaskStatus(task.status) as TaskStatus;

  const handleSelect = async (value: TaskStatus) => {
    setOpen(false);
    if (value === current) return;
    try {
      await mutate(task.id, { status: value }, () => updateStatusOrTranslateGate(task.id, value));
    } catch {
      /* toast already raised by hook */
    }
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          aria-haspopup="listbox"
          aria-expanded={open}
          data-testid="status-picker-trigger"
          className="inline-flex items-center gap-1.5 cursor-pointer rounded px-2 py-1 hover:bg-accent/50 ml-auto"
        >
          <StatusIcon status={current} className="h-3.5 w-3.5" />
          <span>{STATUS_LABELS[current] ?? task.status}</span>
          <IconChevronDown className="h-3 w-3 opacity-50" />
        </button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-48 p-1" portal={false} role="listbox">
        {STATUS_OPTIONS.map((opt) => (
          <button
            key={opt.value}
            type="button"
            role="option"
            aria-selected={current === opt.value}
            data-testid={`status-picker-option-${opt.value}`}
            className={cn(
              "flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm cursor-pointer hover:bg-accent",
              current === opt.value && "bg-accent/40",
            )}
            onClick={() => handleSelect(opt.value)}
          >
            <StatusIcon status={opt.value} className="h-3.5 w-3.5" />
            <span>{opt.label}</span>
          </button>
        ))}
      </PopoverContent>
    </Popover>
  );
}
