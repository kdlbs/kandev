"use client";

import { useEffect, useMemo } from "react";
import { Combobox, type ComboboxOption } from "@/components/combobox";
import { useAppStore } from "@/components/state-provider";
import { updateTask } from "@/lib/api/domains/office-extended-api";
import { listAgentProfiles } from "@/lib/api/domains/office-api";
import { useOptimisticTaskMutation } from "@/hooks/use-optimistic-task-mutation";
import { AgentAvatar } from "@/app/office/components/agent-avatar";
import type { Task } from "@/app/office/tasks/[id]/types";

type AssigneePickerProps = {
  task: Task;
};

const NO_ASSIGNEE = "__none__";

export function AssigneePicker({ task }: AssigneePickerProps) {
  const agents = useAppStore((s) => s.office.agentProfiles);
  const setOfficeAgentProfiles = useAppStore((s) => s.setOfficeAgentProfiles);
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const mutate = useOptimisticTaskMutation();

  // The store is hydrated from /office/inbox and /office/agents pages but
  // not from individual task pages, so navigating directly to a task
  // leaves agentProfiles empty. Lazy-fetch on mount when missing — same
  // pattern the parent/blockers pickers use against searchTasks.
  useEffect(() => {
    if (!workspaceId || agents.length > 0) return;
    let cancelled = false;
    listAgentProfiles(workspaceId)
      .then((res) => {
        if (!cancelled && res.agents) setOfficeAgentProfiles(res.agents);
      })
      .catch(() => {
        /* swallow: picker just shows No assignee */
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId, agents.length, setOfficeAgentProfiles]);

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
