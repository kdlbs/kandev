"use client";

import { Checkbox } from "@kandev/ui/checkbox";
import { IconChevronDown, IconChevronRight } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import {
  defaultGroupExpanded,
  effectiveGroupExpanded,
  shownStepCount,
  type DisclosureOverrides,
} from "@/lib/kanban/steps-disclosure";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import { mobileFieldClass, mobileFieldLabelClass } from "./mobile-menu-styles";

export type StepsVisibilitySectionProps = {
  eligibleWorkflows: Array<{ id: string; name: string }>;
  snapshots: Record<string, WorkflowSnapshotData>;
  hiddenWorkflowStepIds: Record<string, string[]>;
  onToggleStepVisibility: (workflowId: string, stepId: string) => void;
  overrides: DisclosureOverrides;
  onToggleGroupDisclosure: (workflowId: string, defaultValue: boolean) => void;
};

type StepItem = { id: string; title: string; position: number };

function sortedSteps(snapshot: WorkflowSnapshotData | undefined): StepItem[] {
  return [...(snapshot?.steps ?? [])].sort(
    (a, b) => a.position - b.position || a.id.localeCompare(b.id),
  );
}

function StepRow({
  step,
  hidden,
  onToggle,
}: {
  step: StepItem;
  hidden: boolean;
  onToggle: () => void;
}) {
  return (
    <label
      data-testid={`steps-filter-step-row-${step.id}`}
      className="flex min-h-11 cursor-pointer items-center gap-2 px-2"
    >
      <Checkbox
        data-testid={`steps-filter-step-${step.id}`}
        checked={!hidden}
        onCheckedChange={onToggle}
      />
      <span className="min-w-0 flex-1 truncate text-sm text-foreground" title={step.title}>
        {step.title}
      </span>
    </label>
  );
}

function GroupHeader({
  workflowId,
  workflowName,
  expanded,
  shown,
  total,
  onToggle,
}: {
  workflowId: string;
  workflowName: string;
  expanded: boolean;
  shown: number;
  total: number;
  onToggle: () => void;
}) {
  const { t } = useTranslation();
  const Icon = expanded ? IconChevronDown : IconChevronRight;
  return (
    <button
      type="button"
      data-testid={`steps-filter-group-toggle-${workflowId}`}
      aria-expanded={expanded}
      onClick={onToggle}
      className="flex min-h-11 w-full cursor-pointer items-center gap-2 px-2 text-left"
    >
      <span
        className="min-w-0 flex-1 truncate text-sm font-medium text-foreground"
        title={workflowName}
      >
        {workflowName}
      </span>
      <span className="shrink-0 text-xs text-muted-foreground">
        {t("kanban:stepsShownOfTotal", { shown, total })}
      </span>
      <Icon className="h-4 w-4 shrink-0 text-muted-foreground" />
    </button>
  );
}

function StepsGroup({
  workflow,
  snapshot,
  hiddenIds,
  showHeader,
  overrides,
  onToggleStepVisibility,
  onToggleGroupDisclosure,
}: {
  workflow: { id: string; name: string };
  snapshot: WorkflowSnapshotData | undefined;
  hiddenIds: string[];
  showHeader: boolean;
  overrides: DisclosureOverrides;
  onToggleStepVisibility: (workflowId: string, stepId: string) => void;
  onToggleGroupDisclosure: (workflowId: string, defaultValue: boolean) => void;
}) {
  const steps = sortedSteps(snapshot);
  const liveStepIds = new Set(steps.map((s) => s.id));
  const isDefaultExpanded = defaultGroupExpanded(hiddenIds, liveStepIds);
  const isExpanded = showHeader
    ? effectiveGroupExpanded(workflow.id, overrides, isDefaultExpanded)
    : true;
  const { shown, total } = shownStepCount(
    steps.map((s) => s.id),
    hiddenIds,
  );
  const hiddenSet = new Set(hiddenIds);

  return (
    <div className="space-y-1" data-testid={`steps-filter-group-${workflow.id}`}>
      {showHeader && (
        <GroupHeader
          workflowId={workflow.id}
          workflowName={workflow.name}
          expanded={isExpanded}
          shown={shown}
          total={total}
          onToggle={() => onToggleGroupDisclosure(workflow.id, isDefaultExpanded)}
        />
      )}
      {isExpanded &&
        steps.map((step) => (
          <StepRow
            key={step.id}
            step={step}
            hidden={hiddenSet.has(step.id)}
            onToggle={() => onToggleStepVisibility(workflow.id, step.id)}
          />
        ))}
    </div>
  );
}

/**
 * Shared Steps-section body consumed by both the Display dropdown (desktop,
 * tablet) and the mobile menu drawer. Given the same
 * (eligibleWorkflows, snapshots, hiddenWorkflowStepIds, overrides), the
 * rendered group set, step set, ordering, disclosure state, and checked state
 * are identical on both — `overrides` is the one per-surface input, owned by
 * each surface's own owner (never hoisted into shared/store state).
 */
export function StepsVisibilitySection({
  eligibleWorkflows,
  snapshots,
  hiddenWorkflowStepIds,
  onToggleStepVisibility,
  overrides,
  onToggleGroupDisclosure,
}: StepsVisibilitySectionProps) {
  const { t } = useTranslation();
  if (eligibleWorkflows.length === 0) return null;
  const showHeader = eligibleWorkflows.length > 1;

  return (
    <div className={mobileFieldClass} data-testid="steps-filter-section">
      <label className={mobileFieldLabelClass}>{t("kanban:steps")}</label>
      <p className="text-xs text-muted-foreground">{t("kanban:stepsSectionDescription")}</p>
      {eligibleWorkflows.map((wf) => (
        <StepsGroup
          key={wf.id}
          workflow={wf}
          snapshot={snapshots[wf.id]}
          hiddenIds={hiddenWorkflowStepIds[wf.id] ?? []}
          showHeader={showHeader}
          overrides={overrides}
          onToggleStepVisibility={onToggleStepVisibility}
          onToggleGroupDisclosure={onToggleGroupDisclosure}
        />
      ))}
    </div>
  );
}
