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
import { IconCheck, IconChevronDown, IconChevronLeft, IconChevronRight } from "@tabler/icons-react";
import type { WorkflowStep } from "../kanban-column";
import { formatWipCount, isOverWipLimit } from "@/lib/kanban/wip-limit";
import { cn } from "@/lib/utils";

type MobileColumnTabsProps = {
  steps: WorkflowStep[];
  activeIndex: number;
  taskCounts: Record<string, number>;
  onColumnChange: (index: number) => void;
};

function StepCount({ step, count }: { step: WorkflowStep; count: number }) {
  const overWipLimit = isOverWipLimit(count, step.wip_limit);
  const label = formatWipCount(count, step.wip_limit);

  return (
    <Badge
      variant="secondary"
      className={cn(
        "h-5 shrink-0 px-1.5 text-xs tabular-nums",
        overWipLimit && "border-amber-500/50 bg-amber-500/15 text-amber-700 dark:text-amber-300",
      )}
      aria-label={overWipLimit ? `${label} tasks, over WIP limit` : `${label} tasks`}
    >
      {label}
    </Badge>
  );
}

export function MobileColumnTabs({
  steps,
  activeIndex,
  taskCounts,
  onColumnChange,
}: MobileColumnTabsProps) {
  const [open, setOpen] = useState(false);
  const activeStep = steps[activeIndex] ?? steps[0];
  if (!activeStep) return null;

  const selectStep = (index: number) => {
    onColumnChange(index);
    setOpen(false);
  };

  return (
    <Drawer open={open} onOpenChange={setOpen}>
      <div className="grid shrink-0 grid-cols-[44px_minmax(0,1fr)_44px] items-center gap-2 border-b border-border/70 px-4 py-2">
        <Button
          type="button"
          variant="outline"
          size="icon"
          className="h-11 w-11 cursor-pointer rounded-xl transition-[background-color,color,border-color,transform] duration-150 ease-out active:scale-[0.96]"
          disabled={activeIndex === 0}
          onClick={() => onColumnChange(activeIndex - 1)}
          aria-label="Previous step"
        >
          <IconChevronLeft className="h-4 w-4" />
        </Button>

        <DrawerTrigger asChild>
          <Button
            type="button"
            variant="outline"
            className="h-11 min-w-0 cursor-pointer justify-between rounded-xl bg-muted/30 px-3 shadow-sm transition-[background-color,color,border-color,box-shadow,transform] duration-150 ease-out active:scale-[0.96]"
            data-testid="mobile-step-trigger"
          >
            <span className="flex min-w-0 items-center gap-2">
              <span className={cn("h-2.5 w-2.5 shrink-0 rounded-full", activeStep.color)} />
              <span className="truncate font-medium">{activeStep.title}</span>
            </span>
            <span className="flex shrink-0 items-center gap-2">
              <StepCount step={activeStep} count={taskCounts[activeStep.id] ?? 0} />
              <IconChevronDown className="h-4 w-4 text-muted-foreground" />
            </span>
          </Button>
        </DrawerTrigger>

        <Button
          type="button"
          variant="outline"
          size="icon"
          className="h-11 w-11 cursor-pointer rounded-xl transition-[background-color,color,border-color,transform] duration-150 ease-out active:scale-[0.96]"
          disabled={activeIndex === steps.length - 1}
          onClick={() => onColumnChange(activeIndex + 1)}
          aria-label="Next step"
        >
          <IconChevronRight className="h-4 w-4" />
        </Button>
      </div>

      <DrawerContent data-testid="mobile-step-picker" className="max-h-[80dvh]">
        <DrawerHeader className="text-left pb-2">
          <DrawerTitle className="text-balance">Choose step</DrawerTitle>
          <DrawerDescription className="text-pretty">
            Select which workflow step to show.
          </DrawerDescription>
        </DrawerHeader>
        <div className="min-h-0 overflow-y-auto px-2 pb-[max(1rem,env(safe-area-inset-bottom))]">
          {steps.map((step, index) => {
            const isActive = index === activeIndex;
            return (
              <button
                key={step.id}
                type="button"
                data-testid={`column-tab-${index}`}
                data-active={isActive}
                aria-current={isActive ? "step" : undefined}
                onClick={() => selectStep(index)}
                className={cn(
                  "flex min-h-11 w-full cursor-pointer items-center gap-3 rounded-lg px-3 text-left transition-[background-color,transform] duration-150 ease-out active:scale-[0.96]",
                  isActive ? "bg-primary/10 text-foreground" : "hover:bg-muted active:bg-muted",
                )}
              >
                <span className={cn("h-2.5 w-2.5 shrink-0 rounded-full", step.color)} />
                <span className="min-w-0 flex-1 truncate text-sm font-medium">{step.title}</span>
                <StepCount step={step} count={taskCounts[step.id] ?? 0} />
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
