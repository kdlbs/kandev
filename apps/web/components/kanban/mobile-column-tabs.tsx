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
import {
  IconCheck,
  IconChevronDown,
  IconChevronLeft,
  IconChevronRight,
  IconLayoutKanban,
} from "@tabler/icons-react";
import type { WorkflowStep } from "../kanban-column";
import type { MobileWorkflowNavigation } from "@/lib/kanban/view-registry";
import { formatWipCount, isOverWipLimit } from "@/lib/kanban/wip-limit";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";

type MobileColumnTabsProps = {
  steps: WorkflowStep[];
  activeIndex: number;
  taskCounts: Record<string, number>;
  onColumnChange: (index: number) => void;
  workflowNavigation?: MobileWorkflowNavigation;
  allStepsAutoHidden?: boolean;
};

function StepCount({ step, count }: { step: WorkflowStep; count: number }) {
  const { t } = useTranslation();
  const overWipLimit = isOverWipLimit(count, step.wip_limit);
  const label = formatWipCount(count, step.wip_limit);

  return (
    <Badge
      variant="secondary"
      className={cn(
        "h-5 shrink-0 px-1.5 text-xs tabular-nums",
        overWipLimit && "border-amber-500/50 bg-amber-500/15 text-amber-700 dark:text-amber-300",
      )}
      aria-label={
        overWipLimit
          ? t("kanban:tasksOverWipLimit", { label })
          : t("kanban:wipTaskCount", { label })
      }
    >
      {label}
    </Badge>
  );
}

function SelectionCheck({ active }: { active: boolean }) {
  return (
    <IconCheck
      className={cn("h-4 w-4 shrink-0", active ? "opacity-100" : "opacity-0")}
      aria-hidden
    />
  );
}

function WorkflowOptions({
  navigation,
  onSelect,
}: {
  navigation: MobileWorkflowNavigation;
  onSelect: (workflowId: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <section aria-labelledby="mobile-workflow-heading">
      <h3
        id="mobile-workflow-heading"
        className="px-3 pb-1.5 text-xs font-medium uppercase tracking-wide text-muted-foreground"
      >
        {t("kanban:workflow")}
      </h3>
      {navigation.workflows.map((workflow) => {
        const isActive = workflow.id === navigation.activeWorkflowId;
        return (
          <button
            key={workflow.id}
            type="button"
            onClick={() => onSelect(workflow.id)}
            className={cn(
              "flex min-h-11 w-full cursor-pointer items-center gap-3 rounded-lg px-3 text-left transition-[background-color,transform] duration-150 ease-out active:scale-[0.96]",
              isActive ? "bg-primary/10 text-foreground" : "hover:bg-muted active:bg-muted",
            )}
            data-testid={`mobile-workflow-item-${workflow.id}`}
            data-active={isActive}
            aria-current={isActive ? "true" : undefined}
          >
            <IconLayoutKanban className="h-4 w-4 shrink-0 text-muted-foreground" />
            <span className="min-w-0 flex-1 truncate text-sm font-medium">{workflow.name}</span>
            <Badge variant="secondary" className="h-5 shrink-0 px-1.5 tabular-nums">
              {workflow.taskCount}
            </Badge>
            <SelectionCheck active={isActive} />
          </button>
        );
      })}
    </section>
  );
}

function StepOptions({
  steps,
  activeIndex,
  taskCounts,
  onSelect,
  separated,
  allStepsAutoHidden,
}: {
  steps: WorkflowStep[];
  activeIndex: number;
  taskCounts: Record<string, number>;
  onSelect: (index: number) => void;
  separated: boolean;
  allStepsAutoHidden?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <section
      className={cn(separated && "mt-3 border-t border-border/70 pt-3")}
      aria-labelledby="mobile-step-heading"
    >
      <h3
        id="mobile-step-heading"
        className="px-3 pb-1.5 text-xs font-medium uppercase tracking-wide text-muted-foreground"
      >
        {t("kanban:step")}
      </h3>
      {steps.length === 0 && (
        <p className="px-3 py-3 text-sm text-muted-foreground">
          {allStepsAutoHidden ? t("kanban:allEmptyStepsAutoHidden") : t("kanban:noStepsConfigured")}
        </p>
      )}
      {steps.map((step, index) => {
        const isActive = index === activeIndex;
        return (
          <button
            key={step.id}
            type="button"
            data-testid={`column-tab-${index}`}
            data-active={isActive}
            aria-current={isActive ? "step" : undefined}
            onClick={() => onSelect(index)}
            className={cn(
              "flex min-h-11 w-full cursor-pointer items-center gap-3 rounded-lg px-3 text-left transition-[background-color,transform] duration-150 ease-out active:scale-[0.96]",
              isActive ? "bg-primary/10 text-foreground" : "hover:bg-muted active:bg-muted",
            )}
          >
            <span className={cn("h-2.5 w-2.5 shrink-0 rounded-full", step.color)} />
            <span className="min-w-0 flex-1 truncate text-sm font-medium">{step.title}</span>
            <StepCount step={step} count={taskCounts[step.id] ?? 0} />
            <SelectionCheck active={isActive} />
          </button>
        );
      })}
    </section>
  );
}

function NavigatorDrawerContent({
  steps,
  activeIndex,
  taskCounts,
  workflowNavigation,
  allStepsAutoHidden,
  onSelectStep,
  onSelectWorkflow,
}: Omit<MobileColumnTabsProps, "onColumnChange"> & {
  onSelectStep: (index: number) => void;
  onSelectWorkflow: (workflowId: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <DrawerContent data-testid="mobile-board-navigator-drawer" className="max-h-[85dvh]">
      <DrawerHeader className="pb-2 text-left">
        <DrawerTitle className="text-balance">{t("kanban:boardNavigator")}</DrawerTitle>
        <DrawerDescription className="text-pretty">
          {t("kanban:chooseWorkflowAndStepShownOn")}
        </DrawerDescription>
      </DrawerHeader>
      <div className="min-h-0 overflow-y-auto px-2 pb-[max(1rem,env(safe-area-inset-bottom))]">
        {workflowNavigation && (
          <WorkflowOptions navigation={workflowNavigation} onSelect={onSelectWorkflow} />
        )}
        <StepOptions
          steps={steps}
          activeIndex={activeIndex}
          taskCounts={taskCounts}
          onSelect={onSelectStep}
          separated={!!workflowNavigation}
          allStepsAutoHidden={allStepsAutoHidden}
        />
      </div>
    </DrawerContent>
  );
}

function getEmptyStepsLabel(allStepsAutoHidden: boolean, t: (key: string) => string): string {
  return t(allStepsAutoHidden ? "kanban:allEmptyStepsAutoHidden" : "kanban:noStepsConfigured");
}

function PreviousStepButton({
  activeStep,
  activeIndex,
  onColumnChange,
  t,
}: {
  activeStep?: WorkflowStep;
  activeIndex: number;
  onColumnChange: (index: number) => void;
  t: (key: string) => string;
}) {
  return (
    <Button
      type="button"
      variant="outline"
      size="icon"
      className="h-11 w-11 cursor-pointer rounded-xl transition-[background-color,color,border-color,transform] duration-150 ease-out active:scale-[0.96]"
      disabled={!activeStep || activeIndex === 0}
      onClick={() => onColumnChange(activeIndex - 1)}
      aria-label={t("kanban:previousStep")}
    >
      <IconChevronLeft className="h-4 w-4" />
    </Button>
  );
}

function NextStepButton({
  activeStep,
  activeIndex,
  stepCount,
  onColumnChange,
  t,
}: {
  activeStep?: WorkflowStep;
  activeIndex: number;
  stepCount: number;
  onColumnChange: (index: number) => void;
  t: (key: string) => string;
}) {
  return (
    <Button
      type="button"
      variant="outline"
      size="icon"
      className="h-11 w-11 cursor-pointer rounded-xl transition-[background-color,color,border-color,transform] duration-150 ease-out active:scale-[0.96]"
      disabled={!activeStep || activeIndex === stepCount - 1}
      onClick={() => onColumnChange(activeIndex + 1)}
      aria-label={t("kanban:nextStep")}
    >
      <IconChevronRight className="h-4 w-4" />
    </Button>
  );
}

function NavigatorTrigger({
  activeStep,
  activeWorkflow,
  stepLabel,
  taskCounts,
  t,
}: {
  activeStep?: WorkflowStep;
  activeWorkflow?: MobileWorkflowNavigation["workflows"][number];
  stepLabel: string;
  taskCounts: Record<string, number>;
  t: (key: string, options?: Record<string, string>) => string;
}) {
  const ariaLabel = activeWorkflow
    ? t("kanban:chooseWorkflowOrStep", { name: activeWorkflow.name, stepLabel })
    : t("kanban:chooseStep", { stepLabel });
  return (
    <DrawerTrigger asChild>
      <Button
        type="button"
        variant="outline"
        className="h-14 min-w-0 cursor-pointer justify-between rounded-xl bg-muted/30 px-3 shadow-sm transition-[background-color,color,border-color,box-shadow,transform] duration-150 ease-out active:scale-[0.96]"
        data-testid="mobile-board-navigator"
        aria-label={ariaLabel}
      >
        <span className="flex min-w-0 items-center gap-2.5 text-left">
          <IconLayoutKanban className="h-4 w-4 shrink-0 text-muted-foreground" />
          <span className="flex min-w-0 flex-col">
            {activeWorkflow && (
              <span className="truncate text-[11px] font-medium leading-4 text-muted-foreground">
                {activeWorkflow.name}
              </span>
            )}
            <span className="flex min-w-0 items-center gap-1.5">
              {activeStep && (
                <span className={cn("h-2 w-2 shrink-0 rounded-full", activeStep.color)} />
              )}
              <span className="truncate text-sm font-semibold leading-5">{stepLabel}</span>
            </span>
          </span>
        </span>
        <span className="flex shrink-0 items-center gap-2">
          {activeStep && <StepCount step={activeStep} count={taskCounts[activeStep.id] ?? 0} />}
          <IconChevronDown className="h-4 w-4 text-muted-foreground" />
        </span>
      </Button>
    </DrawerTrigger>
  );
}

function MobileColumnTabsHeader({
  steps,
  activeStep,
  activeIndex,
  activeWorkflow,
  stepLabel,
  taskCounts,
  onColumnChange,
  t,
}: {
  steps: WorkflowStep[];
  activeStep?: WorkflowStep;
  activeIndex: number;
  activeWorkflow?: MobileWorkflowNavigation["workflows"][number];
  stepLabel: string;
  taskCounts: Record<string, number>;
  onColumnChange: (index: number) => void;
  t: (key: string, options?: Record<string, string>) => string;
}) {
  return (
    <div className="grid shrink-0 grid-cols-[44px_minmax(0,1fr)_44px] items-center gap-2 border-b border-border/70 px-4 py-2">
      <PreviousStepButton
        activeStep={activeStep}
        activeIndex={activeIndex}
        onColumnChange={onColumnChange}
        t={t}
      />
      <NavigatorTrigger
        activeStep={activeStep}
        activeWorkflow={activeWorkflow}
        stepLabel={stepLabel}
        taskCounts={taskCounts}
        t={t}
      />
      <NextStepButton
        activeStep={activeStep}
        activeIndex={activeIndex}
        stepCount={steps.length}
        onColumnChange={onColumnChange}
        t={t}
      />
    </div>
  );
}

export function MobileColumnTabs({
  steps,
  activeIndex,
  taskCounts,
  onColumnChange,
  workflowNavigation,
  allStepsAutoHidden = false,
}: MobileColumnTabsProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const activeStep = steps[activeIndex] ?? steps[0];
  const activeWorkflow =
    workflowNavigation?.workflows.find(
      (workflow) => workflow.id === workflowNavigation.activeWorkflowId,
    ) ?? workflowNavigation?.workflows[0];
  const stepLabel = activeStep?.title ?? getEmptyStepsLabel(allStepsAutoHidden, t);

  const selectStep = (index: number) => {
    onColumnChange(index);
    setOpen(false);
  };
  const selectWorkflow = (workflowId: string) => {
    workflowNavigation?.onWorkflowChange(workflowId);
  };

  return (
    <Drawer open={open} onOpenChange={setOpen}>
      <MobileColumnTabsHeader
        steps={steps}
        activeStep={activeStep}
        activeIndex={activeIndex}
        activeWorkflow={activeWorkflow}
        stepLabel={stepLabel}
        taskCounts={taskCounts}
        onColumnChange={onColumnChange}
        t={t}
      />

      <NavigatorDrawerContent
        steps={steps}
        activeIndex={activeIndex}
        taskCounts={taskCounts}
        workflowNavigation={workflowNavigation}
        allStepsAutoHidden={allStepsAutoHidden}
        onSelectStep={selectStep}
        onSelectWorkflow={selectWorkflow}
      />
    </Drawer>
  );
}
