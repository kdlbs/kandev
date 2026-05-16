"use client";

import { useEffect, useMemo, useState } from "react";
import { IconX } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import {
  addTaskBlocker,
  removeTaskBlocker,
  searchTasks,
} from "@/lib/api/domains/office-extended-api";
import { ApiError } from "@/lib/api/client";
import { useOptimisticTaskMutation } from "@/hooks/use-optimistic-task-mutation";
import type { OfficeTask } from "@/lib/state/slices/office/types";
import type { Task } from "@/app/office/tasks/[id]/types";
import { MultiSelectPopover, type MultiSelectItem } from "./multi-select-popover";

// formatBlockerCycleMessage renders the toast text for a 400 response
// whose body carries a `cycle` array. The backend already substitutes
// identifiers when available; we just join with the arrow separator and
// prepend a human label.
export function formatBlockerCycleMessage(cycle: string[]): string {
  return `Would create a blocker cycle: ${cycle.join(" → ")}`;
}

// extractCycle reads the `cycle` field from a structured ApiError body.
// Returns null when the error doesn't carry a cycle path so callers
// fall through to the generic error message.
function extractCycle(err: unknown): string[] | null {
  if (!(err instanceof ApiError)) return null;
  const body = err.body;
  if (!body || typeof body !== "object") return null;
  const cycle = (body as { cycle?: unknown }).cycle;
  if (!Array.isArray(cycle)) return null;
  if (!cycle.every((entry): entry is string => typeof entry === "string")) {
    return null;
  }
  return cycle;
}

// addBlockerOrTranslateCycle calls the addTaskBlocker API and rethrows
// any error. When the backend returns a 400 with a `cycle` body, the
// raw error is replaced by an Error whose message is the formatted
// cycle path so the optimistic-mutation hook's default toast surfaces
// a friendly message.
async function addBlockerOrTranslateCycle(taskID: string, blockerID: string): Promise<void> {
  try {
    await addTaskBlocker(taskID, blockerID);
  } catch (err) {
    const cycle = extractCycle(err);
    if (cycle) {
      throw new Error(formatBlockerCycleMessage(cycle));
    }
    throw err;
  }
}

type BlockersPickerProps = {
  task: Task;
};

type BlockerItem = MultiSelectItem & {
  identifier: string;
  title: string;
};

function buildItems(candidates: OfficeTask[], currentTaskId: string): BlockerItem[] {
  return candidates
    .filter((t) => t.id !== currentTaskId)
    .map<BlockerItem>((t) => ({
      id: t.id,
      identifier: t.identifier,
      title: t.title,
      label: `${t.identifier} ${t.title}`,
      keywords: [t.identifier, t.title],
    }));
}

export function BlockersPicker({ task }: BlockersPickerProps) {
  const storeTasks = useAppStore((s) => s.office.tasks.items);
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const [fetched, setFetched] = useState<OfficeTask[]>([]);
  const mutate = useOptimisticTaskMutation();

  useEffect(() => {
    if (!workspaceId || storeTasks.length > 0) return;
    let cancelled = false;
    searchTasks(workspaceId, "", 50)
      .then((res) => {
        if (!cancelled) setFetched(res.tasks ?? []);
      })
      .catch(() => {
        if (!cancelled) setFetched([]);
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId, storeTasks.length]);

  const candidates = storeTasks.length > 0 ? storeTasks : fetched;
  const items = useMemo(() => buildItems(candidates, task.id), [candidates, task.id]);

  const handleAdd = async (id: string) => {
    if (task.blockedBy.includes(id)) return;
    const next = [...task.blockedBy, id];
    try {
      await mutate(task.id, { blockedBy: next }, () => addBlockerOrTranslateCycle(task.id, id));
    } catch {
      /* hook toasts */
    }
  };

  const handleRemove = async (id: string) => {
    if (!task.blockedBy.includes(id)) return;
    const next = task.blockedBy.filter((b) => b !== id);
    try {
      await mutate(task.id, { blockedBy: next }, () => removeTaskBlocker(task.id, id));
    } catch {
      /* hook toasts */
    }
  };

  const renderChip = (item: BlockerItem, remove: () => void) => (
    <span
      key={item.id}
      className="inline-flex items-center gap-1 rounded-full bg-muted px-2 py-0.5 text-xs"
    >
      <span className="font-mono opacity-70">{item.identifier}</span>
      <span className="truncate max-w-[120px]">{item.title}</span>
      <span
        role="button"
        tabIndex={0}
        className="ml-0.5 cursor-pointer opacity-60 hover:opacity-100 inline-flex"
        onClick={(e) => {
          e.stopPropagation();
          remove();
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            e.stopPropagation();
            remove();
          }
        }}
        aria-label={`Remove ${item.identifier}`}
      >
        <IconX className="h-2.5 w-2.5" />
      </span>
    </span>
  );

  const renderItem = (item: BlockerItem) => (
    <span className="flex items-center gap-2 min-w-0">
      <span className="font-mono text-xs text-muted-foreground shrink-0">{item.identifier}</span>
      <span className="truncate">{item.title}</span>
    </span>
  );

  void workspaceId;

  return (
    <MultiSelectPopover
      items={items}
      selectedIds={task.blockedBy}
      onAdd={handleAdd}
      onRemove={handleRemove}
      renderChip={renderChip}
      renderItem={renderItem}
      addLabel="+ Add blocker"
      searchPlaceholder="Search tasks..."
      emptyMessage="No tasks found."
      testId="blockers-picker-trigger"
    />
  );
}
