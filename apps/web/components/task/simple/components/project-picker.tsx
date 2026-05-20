"use client";

import { useMemo } from "react";
import { Combobox, type ComboboxOption } from "@/components/combobox";
import { useAppStore } from "@/components/state-provider";
import { updateTask } from "@/lib/api/domains/office-extended-api";
import { useOptimisticTaskMutation } from "@/hooks/use-optimistic-task-mutation";
import type { Task } from "@/app/office/tasks/[id]/types";

type ProjectPickerProps = {
  task: Task;
};

const NO_PROJECT = "__none__";

export function ProjectPicker({ task }: ProjectPickerProps) {
  const projects = useAppStore((s) => s.office.projects);
  const mutate = useOptimisticTaskMutation();

  const options = useMemo<ComboboxOption[]>(() => {
    const noOpt: ComboboxOption = {
      value: NO_PROJECT,
      label: "No project",
      keywords: ["none"],
      renderLabel: () => <span className="text-muted-foreground">No project</span>,
    };
    const projectOpts = projects.map<ComboboxOption>((p) => ({
      value: p.id,
      label: p.name,
      keywords: [p.name],
      renderLabel: () => (
        <span className="flex items-center gap-2">
          {p.color && (
            <span
              className="h-2.5 w-2.5 rounded-sm shrink-0"
              style={{ backgroundColor: p.color }}
            />
          )}
          <span>{p.name}</span>
        </span>
      ),
    }));
    return [noOpt, ...projectOpts];
  }, [projects]);

  const currentValue = task.projectId || NO_PROJECT;

  const handleSelect = async (next: string) => {
    const sendValue = next === NO_PROJECT || next === "" ? "" : next;
    if (sendValue === (task.projectId ?? "")) return;
    const matched = projects.find((p) => p.id === sendValue);
    try {
      await mutate(
        task.id,
        {
          projectId: sendValue || undefined,
          projectName: matched?.name,
          projectColor: matched?.color,
        },
        () => updateTask(task.id, { project_id: sendValue }),
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
      placeholder="No project"
      searchPlaceholder="Search projects..."
      emptyMessage="No projects found."
      triggerClassName="h-7 w-full justify-end px-2"
      popoverAlign="end"
      testId="project-picker-trigger"
    />
  );
}
