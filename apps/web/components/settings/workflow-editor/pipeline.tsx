"use client";

import { useTranslation } from "react-i18next";
import { IconAlertTriangle, IconChevronRight, IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import type { WorkflowStep } from "@/lib/types/http";
import type {
  WorkflowEditorViewModel,
  WorkflowStepSummary,
} from "@/lib/workflows/workflow-editor-view-model";
import { cn } from "@/lib/utils";

export type WorkflowEditorPipelineProps = {
  steps: WorkflowStep[];
  model: WorkflowEditorViewModel;
  selectedStepId: string | null;
  onSelectStep: (stepId: string) => void;
  onAddStep: () => void;
  readOnly: boolean;
  mobile?: boolean;
};

export function WorkflowEditorPipeline({
  steps,
  model,
  selectedStepId,
  onSelectStep,
  onAddStep,
  readOnly,
  mobile = false,
}: WorkflowEditorPipelineProps) {
  const { t } = useTranslation();
  const summaries = model.stepSummaries;
  return (
    <section
      className={cn("space-y-3", mobile ? "w-full" : "min-w-0")}
      data-testid={mobile ? "workflow-editor-mobile-pipeline" : "workflow-editor-pipeline"}
    >
      <div className="flex items-center justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">{t("workflows:pipeline")}</h2>
          <p className="text-xs text-muted-foreground">{t("workflows:pipelineHelp")}</p>
        </div>
        {!mobile && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="cursor-pointer"
            onClick={readOnly ? undefined : onAddStep}
            disabled={readOnly}
            data-testid="workflow-editor-add-step"
          >
            <IconPlus className="mr-1.5 h-4 w-4" />
            {t("workflows:addStep")}
          </Button>
        )}
      </div>
      {mobile ? (
        <div className="space-y-3" data-testid="workflow-editor-mobile-journey">
          {summaries.map((summary) => (
            <MobileStepCard
              key={summary.stepId}
              summary={summary}
              selected={summary.stepId === selectedStepId}
              onSelect={() => onSelectStep(summary.stepId)}
            />
          ))}
          {!readOnly && (
            <Button
              type="button"
              variant="outline"
              className="min-h-11 w-full cursor-pointer"
              onClick={onAddStep}
            >
              <IconPlus className="mr-1.5 h-4 w-4" />
              {t("workflows:addStep")}
            </Button>
          )}
        </div>
      ) : (
        <div
          className="max-w-full overflow-x-auto pb-2"
          data-testid="workflow-editor-pipeline-scroll"
        >
          <div className="flex min-w-max items-stretch gap-2">
            {summaries.map((summary, index) => (
              <div key={summary.stepId} className="flex items-center gap-2">
                {index > 0 && <PipelineConnector />}
                <DesktopStepCard
                  summary={summary}
                  selected={summary.stepId === selectedStepId}
                  onSelect={() => onSelectStep(summary.stepId)}
                />
              </div>
            ))}
            {steps.length === 0 && (
              <p className="rounded-md border border-dashed border-border px-4 py-5 text-sm text-muted-foreground">
                {t("workflows:noWorkflowSteps")}
              </p>
            )}
          </div>
        </div>
      )}
    </section>
  );
}

function DesktopStepCard({
  summary,
  selected,
  onSelect,
}: {
  summary: WorkflowStepSummary;
  selected: boolean;
  onSelect: () => void;
}) {
  const { t } = useTranslation();
  return (
    <button
      type="button"
      className={cn(
        "flex min-h-28 w-48 cursor-pointer flex-col items-start gap-2 rounded-xl border-2 p-3 text-left transition-colors",
        selected ? "border-primary bg-primary/5" : "border-border bg-card hover:border-primary/50",
      )}
      aria-current={selected ? "step" : undefined}
      onClick={onSelect}
      data-testid={`workflow-editor-step-${summary.stepId}`}
    >
      <StepTitle summary={summary} />
      <StepDetails summary={summary} />
      {summary.primaryDestinationId && (
        <span className="truncate text-[11px] text-muted-foreground">
          {t("workflows:destinationValue", { destination: summary.primaryDestinationId })}
        </span>
      )}
    </button>
  );
}

function MobileStepCard({
  summary,
  selected,
  onSelect,
}: {
  summary: WorkflowStepSummary;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <button
      type="button"
      className={cn(
        "flex min-h-20 w-full cursor-pointer items-center gap-3 rounded-xl border-2 p-3 text-left",
        selected ? "border-primary bg-primary/5" : "border-border bg-card",
      )}
      aria-current={selected ? "step" : undefined}
      onClick={onSelect}
      data-testid={`workflow-editor-mobile-step-${summary.stepId}`}
    >
      <StepTitle summary={summary} />
      <StepDetails summary={summary} />
      <IconChevronRight className="ml-auto h-5 w-5 shrink-0 text-muted-foreground" />
    </button>
  );
}

function StepTitle({ summary }: { summary: WorkflowStepSummary }) {
  return (
    <span className="flex min-w-0 items-center gap-2">
      <span className={cn("h-3 w-3 shrink-0 rounded-full", summary.color)} />
      <span className="min-w-0 truncate text-sm font-medium">{summary.name}</span>
    </span>
  );
}

function StepDetails({ summary }: { summary: WorkflowStepSummary }) {
  const { t } = useTranslation();
  return (
    <span className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-[11px] text-muted-foreground">
      <span>{t("workflows:actionsCount", { count: summary.actionCount })}</span>
      {summary.effectiveProfileId && (
        <span className="max-w-28 truncate">
          {t("workflows:profileValue", { profile: summary.effectiveProfileId })}
        </span>
      )}
      {summary.issues.length > 0 && (
        <span className="inline-flex items-center gap-1 text-destructive">
          <IconAlertTriangle className="h-3.5 w-3.5" />
          {t("workflows:issuesCount", { count: summary.issues.length })}
        </span>
      )}
    </span>
  );
}

function PipelineConnector() {
  return (
    <div className="flex items-center text-muted-foreground/60" aria-hidden="true">
      <div className="h-px w-4 bg-border" />
      <IconChevronRight className="h-4 w-4" />
    </div>
  );
}
