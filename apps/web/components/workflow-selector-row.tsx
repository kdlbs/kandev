"use client";

import { Fragment, memo, useMemo, useState } from "react";
import { IconCheck, IconChevronDown, IconLogicBuffer } from "@tabler/icons-react";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Button } from "@kandev/ui/button";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";

type StepItem = { id: string; title: string; color: string; position: number };

function InlineSteps({ steps }: { steps: StepItem[] }) {
  if (steps.length === 0) return null;
  return (
    <div className="flex items-center gap-1.5 text-xs text-muted-foreground whitespace-nowrap">
      {steps.map((s, i) => (
        <Fragment key={s.id}>
          {i > 0 && <span className="text-muted-foreground/40">→</span>}
          <span className="flex items-center gap-1">
            <span
              className="h-1.5 w-1.5 rounded-full shrink-0"
              style={{ backgroundColor: s.color || "hsl(var(--muted-foreground))" }}
            />
            {s.title}
          </span>
        </Fragment>
      ))}
    </div>
  );
}

type WorkflowSelectorRowProps = {
  workflows: Array<{ id: string; name: string; description?: string | null }>;
  snapshots: Record<string, WorkflowSnapshotData>;
  selectedWorkflowId: string | null;
  onWorkflowChange: (workflowId: string) => void;
};

export const WorkflowSelectorRow = memo(function WorkflowSelectorRow({
  workflows,
  snapshots,
  selectedWorkflowId,
  onWorkflowChange,
}: WorkflowSelectorRowProps) {
  const [open, setOpen] = useState(false);

  const selectedWorkflow = useMemo(
    () => workflows.find((w) => w.id === selectedWorkflowId),
    [workflows, selectedWorkflowId],
  );

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button type="button" variant="ghost" className="w-auto justify-between cursor-pointer">
          <IconLogicBuffer className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
          <span className="truncate">{selectedWorkflow?.name ?? "Select workflow"}</span>
          <IconChevronDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-auto min-w-[300px] max-w-none p-1" align="start">
        <div className="text-muted-foreground px-2 py-1.5 text-xs border-b">Workflow</div>
        {workflows.map((wf) => {
          const isSelected = wf.id === selectedWorkflowId;
          const snapshot = snapshots[wf.id];
          const steps = snapshot ? [...snapshot.steps].sort((a, b) => a.position - b.position) : [];
          return (
            <button
              key={wf.id}
              type="button"
              onClick={() => {
                onWorkflowChange(wf.id);
                setOpen(false);
              }}
              className="w-full flex flex-col gap-1 px-2 py-1.5 rounded-sm hover:bg-muted transition-colors cursor-pointer text-left relative pr-8"
            >
              <div className="flex items-center gap-2">
                <IconLogicBuffer className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                <span className="text-sm">{wf.name}</span>
              </div>
              {steps.length > 0 && (
                <div className="pl-[calc(0.875rem+0.5rem)]">
                  <InlineSteps steps={steps} />
                </div>
              )}
              {isSelected && (
                <IconCheck className="absolute right-2 top-1/2 -translate-y-1/2 h-4 w-4" />
              )}
            </button>
          );
        })}
      </PopoverContent>
    </Popover>
  );
});
