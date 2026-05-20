"use client";

import { useEffect, useMemo, useState } from "react";
import { Combobox, type ComboboxOption } from "@/components/combobox";
import { useAppStore } from "@/components/state-provider";
import { searchTasks, updateTask } from "@/lib/api/domains/office-extended-api";
import { useOptimisticTaskMutation } from "@/hooks/use-optimistic-task-mutation";
import type { OfficeTask } from "@/lib/state/slices/office/types";
import type { Task } from "@/app/office/tasks/[id]/types";

type ParentPickerProps = {
  task: Task;
};

const NO_PARENT = "__none__";

function buildOptions(candidates: OfficeTask[], currentTaskId: string): ComboboxOption[] {
  const noOpt: ComboboxOption = {
    value: NO_PARENT,
    label: "No parent",
    keywords: ["none"],
    renderLabel: () => <span className="text-muted-foreground">No parent</span>,
  };
  const taskOpts = candidates
    .filter((t) => t.id !== currentTaskId)
    .map<ComboboxOption>((t) => ({
      value: t.id,
      label: `${t.identifier} ${t.title}`,
      keywords: [t.identifier, t.title],
      renderLabel: () => (
        <span className="flex items-center gap-2 min-w-0">
          <span className="font-mono text-xs text-muted-foreground shrink-0">{t.identifier}</span>
          <span className="truncate">{t.title}</span>
        </span>
      ),
    }));
  return [noOpt, ...taskOpts];
}

export function ParentPicker({ task }: ParentPickerProps) {
  const storeTasks = useAppStore((s) => s.office.tasks.items);
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const [fetched, setFetched] = useState<OfficeTask[]>([]);
  const mutate = useOptimisticTaskMutation();

  // If the store doesn't already have tasks for the workspace, lazily fetch.
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

  const options = useMemo(() => buildOptions(candidates, task.id), [candidates, task.id]);

  const currentValue = task.parentId || NO_PARENT;

  const handleSelect = async (next: string) => {
    const sendValue = next === NO_PARENT || next === "" ? "" : next;
    if (sendValue === (task.parentId ?? "")) return;
    const matched = candidates.find((t) => t.id === sendValue);
    try {
      await mutate(
        task.id,
        {
          parentId: sendValue || undefined,
          parentTitle: matched?.title,
          parentIdentifier: matched?.identifier,
        },
        () => updateTask(task.id, { parent_id: sendValue }),
      );
    } catch {
      /* toast already raised */
    }
  };

  return (
    <Combobox
      options={options}
      value={currentValue}
      onValueChange={handleSelect}
      placeholder="No parent"
      searchPlaceholder="Search tasks..."
      emptyMessage="No tasks found."
      triggerClassName="h-7 w-full justify-end px-2"
      popoverAlign="end"
      testId="parent-picker-trigger"
    />
  );
}
