"use client";

import { IconCheck, IconChevronDown } from "@tabler/icons-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { cn } from "@/lib/utils";

type Workflow = {
  id: string;
  name: string;
};

type WorkflowSwitcherProps = {
  workflows: Workflow[];
  activeWorkflowId: string | null;
  onSelect: (workflowId: string) => void;
};

export function WorkflowSwitcher({ workflows, activeWorkflowId, onSelect }: WorkflowSwitcherProps) {
  const selectedWorkflow = workflows.find((w) => w.id === activeWorkflowId);

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          title="Switch Workflows"
          className={cn(
            "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-xs cursor-pointer",
            "text-foreground hover:bg-foreground/5 transition-colors duration-150",
            "focus:outline-none focus-visible:ring-2 focus-visible:ring-ring",
          )}
        >
          {/* Workflow Avatar */}
          <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded bg-foreground/10 text-xs font-medium">
            {selectedWorkflow?.name?.charAt(0) || "W"}
          </span>
          {/* Workflow Name */}
          <span className="flex-1 truncate text-left font-medium">
            {selectedWorkflow?.name || "Select workflow"}
          </span>
          <IconChevronDown className="h-3.5 w-3.5 shrink-0 opacity-50" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-56">
        {workflows.map((workflow) => (
          <DropdownMenuItem
            key={workflow.id}
            onClick={() => onSelect(workflow.id)}
            className={cn(
              "justify-between",
              activeWorkflowId === workflow.id && "bg-foreground/10",
            )}
          >
            <div className="flex items-center gap-2">
              <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded bg-foreground/10 text-xs font-medium">
                {workflow.name.charAt(0)}
              </span>
              <span className="truncate">{workflow.name}</span>
            </div>
            {activeWorkflowId === workflow.id && <IconCheck className="h-4 w-4 shrink-0" />}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
