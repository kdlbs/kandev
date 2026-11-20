"use client";

import { Fragment } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronRight } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { cn } from "@/lib/utils";
import type { Workflow, WorkflowStep } from "@/lib/types/http";

export type WorkflowReadOnlyReason = "synced" | "improve" | null;

type WorkflowCompactRowProps = {
  workflow: Workflow;
  steps: WorkflowStep[];
  stepsLoading: boolean;
  agentProfileLabel?: string;
  readOnlyReason: WorkflowReadOnlyReason;
  isDirty: boolean;
  expanded: boolean;
  onToggle: () => void;
  bodyId: string;
};

function WorkflowCompactMarkers({
  readOnlyReason,
  isDirty,
  descriptionId,
}: Pick<WorkflowCompactRowProps, "readOnlyReason" | "isDirty"> & { descriptionId: string }) {
  const { t } = useTranslation();
  if (!readOnlyReason && !isDirty) return null;
  return (
    <span id={descriptionId} className="flex shrink-0 items-center gap-2">
      {readOnlyReason === "synced" && (
        <Badge variant="outline" className="shrink-0 text-xs">
          {t("workflows:synced")}
        </Badge>
      )}
      {readOnlyReason === "improve" && (
        <Badge variant="outline" className="shrink-0 text-xs">
          {t("workflows:compactReadOnly")}
        </Badge>
      )}
      {isDirty && (
        <Badge variant="secondary" className="shrink-0 text-xs">
          {t("workflows:compactUnsaved")}
        </Badge>
      )}
    </span>
  );
}

function WorkflowCompactSteps({
  steps,
  stepsLoading,
}: {
  steps: WorkflowStep[];
  stepsLoading: boolean;
}) {
  const { t } = useTranslation();
  if (stepsLoading) {
    return (
      <span className="min-w-0 flex-1 truncate text-muted-foreground">
        {t("workflows:loadingWorkflowSteps")}
      </span>
    );
  }
  const ordered = [...steps].sort((a, b) => a.position - b.position);
  return (
    <span
      className="flex min-w-0 flex-1 items-center gap-1 overflow-hidden whitespace-nowrap [mask-image:linear-gradient(to_right,#000_calc(100%-1.5rem),transparent)]"
      data-testid="workflow-compact-steps"
    >
      {ordered.map((step, index) => (
        <Fragment key={step.id}>
          {index > 0 && (
            <IconChevronRight
              className="h-3 w-3 shrink-0 text-muted-foreground"
              aria-hidden="true"
            />
          )}
          <span className="flex shrink-0 items-center gap-1">
            <span className={cn("h-2 w-2 shrink-0 rounded-full", step.color)} aria-hidden="true" />
            <span data-testid="workflow-compact-step">{step.name}</span>
          </span>
        </Fragment>
      ))}
    </span>
  );
}

/**
 * Collapsed one-line summary of a workflow in the Workflows settings list.
 * The whole header is the expand/collapse button; markers are plain badges
 * because interactive content cannot nest inside a button.
 */
export function WorkflowCompactRow({
  workflow,
  steps,
  stepsLoading,
  agentProfileLabel,
  readOnlyReason,
  isDirty,
  expanded,
  onToggle,
  bodyId,
}: WorkflowCompactRowProps) {
  const { t } = useTranslation();
  const summaryId = `workflow-compact-summary-${workflow.id}`;
  const markersId = `workflow-compact-markers-${workflow.id}`;
  const markerDescription = readOnlyReason || isDirty ? ` ${markersId}` : "";
  return (
    <button
      type="button"
      onClick={onToggle}
      aria-expanded={expanded}
      aria-controls={bodyId}
      aria-describedby={`${summaryId}${markerDescription}`}
      aria-label={t("workflows:toggleWorkflowDetails", { name: workflow.name })}
      data-testid={`workflow-compact-row-${workflow.id}`}
      className="flex min-h-10 w-full min-w-0 cursor-pointer flex-col gap-1 rounded-xl py-2 pl-3 pr-14 text-left text-sm transition-colors hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring md:flex-row md:items-center md:gap-3 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
    >
      <span className="flex min-w-0 items-center gap-2 md:max-w-[35%] md:shrink-0">
        <IconChevronRight
          className={cn(
            "h-4 w-4 shrink-0 text-muted-foreground transition-transform",
            expanded && "rotate-90",
          )}
          aria-hidden="true"
        />
        <span className="min-w-0 truncate font-medium">{workflow.name}</span>
        <WorkflowCompactMarkers
          readOnlyReason={readOnlyReason}
          isDirty={isDirty}
          descriptionId={markersId}
        />
      </span>
      {workflow.description && (
        <span className="hidden min-w-0 max-w-[30%] truncate text-muted-foreground lg:block">
          {workflow.description}
        </span>
      )}
      <span id={summaryId} className="flex min-w-0 flex-1 items-center gap-3 pl-6 text-xs md:pl-0">
        <WorkflowCompactSteps steps={steps} stepsLoading={stepsLoading} />
        {!stepsLoading && (
          <span className="shrink-0 text-muted-foreground">
            {t("workflows:compactStepCount", { count: steps.length })}
          </span>
        )}
        {agentProfileLabel && (
          <Badge
            variant="outline"
            className="max-w-20 shrink-0 truncate text-xs md:max-w-40"
            data-testid="workflow-compact-profile"
          >
            {agentProfileLabel}
          </Badge>
        )}
      </span>
    </button>
  );
}
