"use client";

import { useMemo } from "react";
import { Combobox, type ComboboxOption } from "@/components/combobox";
import { updateTask } from "@/lib/api/domains/office-extended-api";
import { useOptimisticTaskMutation } from "@/hooks/use-optimistic-task-mutation";
import { useOfficeAgents } from "@/hooks/domains/office/use-office-agents";
import { AgentAvatar } from "@/app/office/components/agent-avatar";
import type { Task } from "@/app/office/tasks/[id]/types";

type AssigneePickerProps = {
  task: Task;
};

const NO_ASSIGNEE = "__none__";

export function AssigneePicker({ task }: AssigneePickerProps) {
  // Agents come from TanStack Query scoped to the active workspace; the
  // query fetches on demand so navigating straight to a task page still
  // populates the picker (no manual lazy-fetch needed).
  const agents = useOfficeAgents();
  const mutate = useOptimisticTaskMutation();

  const options = useMemo<ComboboxOption[]>(() => {
    const noOpt: ComboboxOption = {
      value: NO_ASSIGNEE,
      label: "No assignee",
      keywords: ["none", "unassigned"],
      renderLabel: () => <span className="text-muted-foreground">No assignee</span>,
    };
    const agentOpts = agents.map<ComboboxOption>((a) => ({
      value: a.id,
      label: a.name,
      keywords: [a.name, a.role ?? ""],
      renderLabel: () => (
        <span className="flex items-center gap-2 min-w-0">
          <AgentAvatar role={a.role} name={a.name} size="sm" />
          <span className="truncate">{a.name}</span>
        </span>
      ),
    }));
    return [noOpt, ...agentOpts];
  }, [agents]);

  const currentValue = task.assigneeAgentProfileId || NO_ASSIGNEE;

  const handleSelect = async (next: string) => {
    const sendValue = next === NO_ASSIGNEE || next === "" ? "" : next;
    if (sendValue === (task.assigneeAgentProfileId ?? "")) return;
    const matchedAgent = agents.find((a) => a.id === sendValue);
    try {
      await mutate(
        task.id,
        {
          assigneeAgentProfileId: sendValue || undefined,
          assigneeName: matchedAgent?.name,
        },
        () => updateTask(task.id, { assignee_agent_profile_id: sendValue }),
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
      placeholder="No assignee"
      searchPlaceholder="Search agents..."
      emptyMessage="No agents found."
      triggerClassName="h-7 w-full justify-end px-2"
      popoverAlign="end"
      testId="assignee-picker-trigger"
    />
  );
}
