"use client";

import { Fragment, memo, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconCheck, IconChevronDown, IconInfoCircle, IconLogicBuffer } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Button } from "@kandev/ui/button";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import type { AgentProfileOption } from "@/lib/state/slices";
import { AgentLogo } from "@/components/agent-logo";
import type { TaskCreateLaunchPreview } from "@/components/task-create-dialog-launch-preview";

type StepItem = {
  id: string;
  title: string;
  color: string;
  position: number;
  agent_profile_id?: string;
  is_start_step?: boolean;
};

function InlineSteps({
  steps,
  agentProfiles,
}: {
  steps: StepItem[];
  agentProfiles: AgentProfileOption[];
}) {
  const { t } = useTranslation();
  if (steps.length === 0) return null;
  return (
    <div className="flex items-center gap-1.5 text-xs text-muted-foreground whitespace-nowrap">
      {steps.map((s, i) => {
        const stepProfile = s.agent_profile_id
          ? agentProfiles.find((p) => p.id === s.agent_profile_id)
          : null;
        return (
          <Fragment key={s.id}>
            {i > 0 && <span className="text-muted-foreground/40">{"\u2192"}</span>}
            <span className="flex items-center gap-1">
              <span
                className="h-1.5 w-1.5 rounded-full shrink-0"
                style={{ backgroundColor: s.color || "hsl(var(--muted-foreground))" }}
              />
              {s.title}
              {s.is_start_step && (
                <TooltipProvider>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <span className="text-[10px] text-muted-foreground/60 leading-none">*</span>
                    </TooltipTrigger>
                    <TooltipContent>{t("workflows:startStep")}</TooltipContent>
                  </Tooltip>
                </TooltipProvider>
              )}
              {stepProfile && (
                <TooltipProvider>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <span data-testid="step-agent-logo">
                        <AgentLogo
                          agentName={stepProfile.agent_name}
                          size={12}
                          className="shrink-0"
                        />
                      </span>
                    </TooltipTrigger>
                    <TooltipContent>{stepProfile.label}</TooltipContent>
                  </Tooltip>
                </TooltipProvider>
              )}
            </span>
          </Fragment>
        );
      })}
    </div>
  );
}

type WorkflowSelectorRowProps = {
  workflows: Array<{
    id: string;
    name: string;
    description?: string | null;
    agent_profile_id?: string;
  }>;
  snapshots: Record<string, WorkflowSnapshotData>;
  selectedWorkflowId: string | null;
  onWorkflowChange: (workflowId: string) => void;
  agentProfiles: AgentProfileOption[];
  launchPreview?: TaskCreateLaunchPreview | null;
  clearLabel?: string;
  placeholder?: string;
};

function WorkflowSelectorTrigger({
  selectedWorkflow,
  placeholder,
}: {
  selectedWorkflow: WorkflowSelectorRowProps["workflows"][number] | undefined;
  placeholder?: string;
}) {
  const { t } = useTranslation();
  return (
    <PopoverTrigger asChild>
      <Button
        type="button"
        variant="ghost"
        className="min-h-11 w-auto min-w-0 max-w-full justify-between cursor-pointer md:min-h-7"
        data-testid="workflow-selector-trigger"
      >
        <IconLogicBuffer className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        <span className="min-w-0 truncate">
          {selectedWorkflow?.name ?? placeholder ?? t("workflows:selectWorkflow")}
        </span>
        <IconChevronDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
      </Button>
    </PopoverTrigger>
  );
}

function LaunchDestinationInfo() {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-6 w-6 shrink-0 cursor-pointer text-muted-foreground hover:text-foreground [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11"
          aria-label={t("task:launchDestinationHelpLabel")}
          data-testid="task-create-launch-step-info"
        >
          <IconInfoCircle className="h-3.5 w-3.5" aria-hidden="true" />
        </Button>
      </TooltipTrigger>
      <TooltipContent className="max-w-[320px] text-xs leading-relaxed">
        {t("task:launchDestinationHelp")}
      </TooltipContent>
    </Tooltip>
  );
}

function LaunchDestinationLabel({ stepName }: { stepName: string }) {
  const { t } = useTranslation();
  return (
    <span
      className="min-w-0 max-w-[45vw] shrink truncate text-xs text-muted-foreground"
      data-testid="task-create-launch-step"
    >
      {t("task:launchDestination", { step: stepName })}
    </span>
  );
}

export const WorkflowSelectorRow = memo(function WorkflowSelectorRow({
  workflows,
  snapshots,
  selectedWorkflowId,
  onWorkflowChange,
  agentProfiles,
  launchPreview,
  clearLabel,
  placeholder,
}: WorkflowSelectorRowProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);

  const selectedWorkflow = useMemo(
    () => workflows.find((w) => w.id === selectedWorkflowId),
    [workflows, selectedWorkflowId],
  );

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <div className="flex min-w-0 items-center gap-2" data-testid="workflow-selector-row">
        <WorkflowSelectorTrigger selectedWorkflow={selectedWorkflow} placeholder={placeholder} />
        {launchPreview && (
          <>
            <LaunchDestinationInfo />
            <LaunchDestinationLabel stepName={launchPreview.stepName} />
          </>
        )}
      </div>
      <PopoverContent className="w-auto min-w-[300px] max-w-none p-1" align="start">
        <div className="text-muted-foreground px-2 py-1.5 text-xs border-b">
          {t("workflows:workflow")}
        </div>
        {clearLabel ? (
          <button
            type="button"
            onClick={() => {
              onWorkflowChange("");
              setOpen(false);
            }}
            className="min-h-11 w-full rounded-sm px-2 py-1.5 text-left text-sm hover:bg-muted"
          >
            {clearLabel}
          </button>
        ) : null}
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
              className="relative flex min-h-11 w-full cursor-pointer flex-col gap-1 rounded-sm px-2 py-1.5 pr-8 text-left transition-colors hover:bg-muted"
            >
              <div className="flex items-center gap-2">
                <IconLogicBuffer className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                <span className="text-sm">{wf.name}</span>
                {wf.agent_profile_id &&
                  (() => {
                    const wfProfile = agentProfiles.find((p) => p.id === wf.agent_profile_id);
                    return wfProfile ? (
                      <TooltipProvider>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <span data-testid="workflow-agent-logo">
                              <AgentLogo
                                agentName={wfProfile.agent_name}
                                size={14}
                                className="shrink-0"
                              />
                            </span>
                          </TooltipTrigger>
                          <TooltipContent>{wfProfile.label}</TooltipContent>
                        </Tooltip>
                      </TooltipProvider>
                    ) : null;
                  })()}
              </div>
              {steps.length > 0 && (
                <div className="pl-[calc(0.875rem+0.5rem)]">
                  <InlineSteps steps={steps} agentProfiles={agentProfiles} />
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
