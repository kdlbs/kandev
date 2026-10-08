"use client";

import { useTranslation } from "react-i18next";
import { IconChevronRight } from "@tabler/icons-react";
import type { WorkflowStep } from "@/lib/types/http";
import {
  getWorkflowActionCatalog,
  type WorkflowActionRecord,
  type WorkflowActionDescriptor,
} from "@/lib/workflows/workflow-action-catalog";
export function WorkflowActionRow({
  action,
  index,
  step,
  steps,
  readOnly,
  onSelect,
}: {
  action: WorkflowActionRecord;
  index: number;
  step: WorkflowStep;
  steps: WorkflowStep[];
  readOnly: boolean;
  onSelect: () => void;
}) {
  const { t } = useTranslation();
  const descriptor = [
    ...getWorkflowActionCatalog("on_enter"),
    ...getWorkflowActionCatalog("on_turn_start"),
    ...getWorkflowActionCatalog("on_turn_complete"),
    ...getWorkflowActionCatalog("on_exit"),
    ...getWorkflowActionCatalog("on_children_completed"),
  ].find((item) => item.type === action.type);
  const summary = actionSummary(action, descriptor, t, step, steps);
  return (
    <button
      type="button"
      className="flex min-h-11 w-full cursor-pointer items-center gap-2 rounded-md border border-border px-3 text-left text-sm transition-colors hover:border-primary/50 md:min-h-9 [@media(pointer:coarse)]:min-h-11"
      aria-label={t("workflows:selectActionNumber", { index: index + 1 })}
      onClick={onSelect}
      data-read-only={readOnly}
    >
      <span className="w-5 shrink-0 text-xs tabular-nums text-muted-foreground">{index + 1}</span>
      <span className="min-w-0 flex-1 truncate font-mono">{summary}</span>
      <IconChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden="true" />
    </button>
  );
}

function actionSummary(
  action: WorkflowActionRecord,
  descriptor: WorkflowActionDescriptor | undefined,
  translate: (key: string, values?: Record<string, string>) => string,
  step: WorkflowStep,
  steps: WorkflowStep[],
): string {
  if (["move_to_step", "move_to_next", "move_to_previous"].includes(action.type)) {
    const ordered = [...steps].sort((a, b) => a.position - b.position);
    const index = ordered.findIndex((item) => item.id === step.id);
    const destination =
      action.type === "move_to_step"
        ? steps.find((item) => item.id === action.config?.step_id)
        : ordered[index + (action.type === "move_to_next" ? 1 : -1)];
    return destination
      ? translate("workflows:stepActionDestination", { step: destination.name })
      : translate("workflows:stepDestinationUnavailable");
  }
  if (action.type === "run_script" && typeof action.config?.command === "string") {
    return action.config.command;
  }
  if (descriptor) return translate(descriptor.labelKey);
  return action.type;
}
