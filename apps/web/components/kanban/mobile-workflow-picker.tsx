"use client";

import { useState } from "react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { IconCheck, IconChevronDown, IconLayoutKanban } from "@tabler/icons-react";
import { cn } from "@/lib/utils";

export type MobileWorkflowOption = {
  id: string;
  name: string;
  taskCount: number;
};

type MobileWorkflowPickerProps = {
  workflows: MobileWorkflowOption[];
  activeWorkflowId: string;
  onWorkflowChange: (workflowId: string) => void;
};

function taskCountLabel(count: number): string {
  return `${count} ${count === 1 ? "task" : "tasks"}`;
}

export function MobileWorkflowPicker({
  workflows,
  activeWorkflowId,
  onWorkflowChange,
}: MobileWorkflowPickerProps) {
  const [open, setOpen] = useState(false);
  const activeWorkflow = workflows.find((workflow) => workflow.id === activeWorkflowId);
  if (!activeWorkflow) return null;

  const selectWorkflow = (workflowId: string) => {
    onWorkflowChange(workflowId);
    setOpen(false);
  };

  return (
    <Drawer open={open} onOpenChange={setOpen}>
      <div className="shrink-0 border-b border-border/70 px-4 py-2">
        <DrawerTrigger asChild>
          <Button
            type="button"
            variant="outline"
            className="h-11 w-full cursor-pointer justify-between rounded-xl bg-muted/30 px-3 shadow-sm transition-[background-color,color,border-color,box-shadow,transform] duration-150 ease-out active:scale-[0.96]"
            data-testid="mobile-workflow-trigger"
          >
            <span className="flex min-w-0 items-center gap-2">
              <IconLayoutKanban className="h-4 w-4 shrink-0 text-muted-foreground" />
              <span className="truncate font-medium">{activeWorkflow.name}</span>
            </span>
            <span className="flex shrink-0 items-center gap-2">
              <Badge variant="secondary" className="h-5 px-1.5 tabular-nums">
                {activeWorkflow.taskCount}
              </Badge>
              <IconChevronDown className="h-4 w-4 text-muted-foreground" />
            </span>
          </Button>
        </DrawerTrigger>
      </div>

      <DrawerContent data-testid="mobile-workflow-picker" className="max-h-[80dvh]">
        <DrawerHeader className="text-left pb-2">
          <DrawerTitle className="text-balance">Choose workflow</DrawerTitle>
          <DrawerDescription className="text-pretty">
            Show one workflow while keeping All workflows selected.
          </DrawerDescription>
        </DrawerHeader>
        <div className="min-h-0 overflow-y-auto px-2 pb-[max(1rem,env(safe-area-inset-bottom))]">
          {workflows.map((workflow) => {
            const isActive = workflow.id === activeWorkflowId;
            return (
              <button
                key={workflow.id}
                type="button"
                onClick={() => selectWorkflow(workflow.id)}
                className={cn(
                  "flex min-h-11 w-full cursor-pointer items-center gap-3 rounded-lg px-3 text-left transition-[background-color,transform] duration-150 ease-out active:scale-[0.96]",
                  isActive ? "bg-primary/10 text-foreground" : "hover:bg-muted active:bg-muted",
                )}
                data-testid={`mobile-workflow-item-${workflow.id}`}
                data-active={isActive}
                aria-current={isActive ? "true" : undefined}
                aria-label={`${workflow.name}, ${taskCountLabel(workflow.taskCount)}`}
              >
                <IconLayoutKanban className="h-4 w-4 shrink-0 text-muted-foreground" />
                <span className="min-w-0 flex-1 truncate text-sm font-medium">{workflow.name}</span>
                <Badge variant="secondary" className="h-5 shrink-0 px-1.5 tabular-nums">
                  {workflow.taskCount}
                </Badge>
                <IconCheck
                  className={cn("h-4 w-4 shrink-0", isActive ? "opacity-100" : "opacity-0")}
                  aria-hidden
                />
              </button>
            );
          })}
        </div>
      </DrawerContent>
    </Drawer>
  );
}
