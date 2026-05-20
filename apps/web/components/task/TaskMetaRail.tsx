"use client";

/**
 * TaskMetaRail — right-rail strategy switch.
 *
 * Branches on (workflow.style, step.stage_type) and renders the
 * appropriate meta surface:
 *
 *   review/approval stage  -> <MultiAgentMeta>     (participants chips, decisions)
 *   workflow.style=office  -> <OfficeMeta>         (stage indicator, reviewers/approvers)
 *   otherwise              -> <WorkflowMeta>       (step picker, prompt preview)
 *
 * Backend follow-up (tracked in plan.md F7): `workflow.style` and
 * `step.stage_type` are not yet exposed on the frontend Workflow /
 * WorkflowStep types. Callers can pass them explicitly here so the rail
 * branches correctly even before the API additions land. Without those
 * hints we fall back to the workflow-meta default (today's behaviour).
 */

import type { ReactNode } from "react";

export type WorkflowStyle = "kanban" | "office" | "custom";
export type StageType = "work" | "review" | "approval" | "custom";

export type TaskMetaRailProps = {
  workflowStyle?: WorkflowStyle | null;
  stageType?: StageType | null;
  /** Multi-agent panel content (participants, decisions). */
  multiAgentSlot?: ReactNode;
  /** Office stage / reviewers / approvers / budget panel. */
  officeSlot?: ReactNode;
  /** Default workflow-meta panel (step picker, prompt preview). */
  workflowSlot?: ReactNode;
};

/**
 * Pure decision function — exported for tests so we don't have to
 * mount React just to assert which branch fires.
 */
export function pickMetaRailVariant(
  style: WorkflowStyle | null | undefined,
  stage: StageType | null | undefined,
): "multi-agent" | "office" | "workflow" {
  if (stage === "review" || stage === "approval") return "multi-agent";
  if (style === "office") return "office";
  return "workflow";
}

export function TaskMetaRail({
  workflowStyle,
  stageType,
  multiAgentSlot,
  officeSlot,
  workflowSlot,
}: TaskMetaRailProps) {
  const variant = pickMetaRailVariant(workflowStyle, stageType);
  if (variant === "multi-agent") return <>{multiAgentSlot ?? null}</>;
  if (variant === "office") return <>{officeSlot ?? null}</>;
  return <>{workflowSlot ?? null}</>;
}
