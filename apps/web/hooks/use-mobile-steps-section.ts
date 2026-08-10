"use client";

import { useStepsDisclosureOverrides } from "./use-steps-disclosure-overrides";
import type { StepsVisibilitySectionProps } from "@/components/kanban/steps-visibility-section";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";

type UseMobileStepsSectionInput = {
  open: boolean;
  breakpoint: string;
  isMobile: boolean;
  currentPage: "kanban" | "tasks";
  eligibleWorkflows: Array<{ id: string; name: string }>;
  snapshots: Record<string, WorkflowSnapshotData>;
  hiddenWorkflowStepIds: Record<string, string[]>;
  onToggleStepVisibility: (workflowId: string, stepId: string) => void;
};

/**
 * Builds the phone drawer's Steps-section props, or `null` when the section
 * is not this surface's to render this render (surface exclusivity — the
 * dropdown owns it off the phone, or on the tasks page neither surface
 * renders it; see `steps-visibility-section.tsx`).
 */
export function useMobileStepsSection({
  open,
  breakpoint,
  isMobile,
  currentPage,
  eligibleWorkflows,
  snapshots,
  hiddenWorkflowStepIds,
  onToggleStepVisibility,
}: UseMobileStepsSectionInput): StepsVisibilitySectionProps | null {
  const { overrides, toggleDisclosure } = useStepsDisclosureOverrides(open, breakpoint);
  if (!(isMobile && currentPage === "kanban")) return null;
  return {
    eligibleWorkflows,
    snapshots,
    hiddenWorkflowStepIds,
    onToggleStepVisibility,
    overrides,
    onToggleGroupDisclosure: toggleDisclosure,
  };
}
