"use client";

import { useState } from "react";
import type { WorkflowStep } from "@/lib/types/http";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { WorkflowInspector, type WorkflowInspectorTab } from "./workflow-editor/inspector";
import { isWorkflowStepDirty } from "./workflow-dirty-state";

type StepConfigPanelProps = {
  step: WorkflowStep;
  savedStep?: WorkflowStep;
  steps: WorkflowStep[];
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  onRemove: () => void;
  readOnly?: boolean;
  onSessionConfigResolutionPendingChange?: (pending: boolean) => void;
};

export function StepConfigPanel({
  step,
  savedStep,
  steps,
  onUpdate,
  onRemove,
  readOnly = false,
  onSessionConfigResolutionPendingChange,
}: StepConfigPanelProps) {
  const [activeTab, setActiveTab] = useState<WorkflowInspectorTab>("agent");
  const { isMobile } = useResponsiveBreakpoint();

  return (
    <div
      className="min-w-0 animate-in fade-in-0 slide-in-from-top-2 duration-200"
      data-settings-dirty={isWorkflowStepDirty(step, savedStep)}
      data-settings-dirty-level="container"
      data-testid={`workflow-step-panel-${step.id}`}
    >
      <WorkflowInspector
        step={step}
        savedStep={savedStep}
        steps={steps}
        activeTab={activeTab}
        readOnly={readOnly}
        onTabChange={setActiveTab}
        onUpdate={onUpdate}
        onRemove={onRemove}
        mobile={isMobile}
        onSessionConfigResolutionPendingChange={onSessionConfigResolutionPendingChange}
      />
    </div>
  );
}
