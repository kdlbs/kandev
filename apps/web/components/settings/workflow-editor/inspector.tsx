"use client";

import { useTranslation } from "react-i18next";
import type { WorkflowStep } from "@/lib/types/http";
import { useAppStore } from "@/components/state-provider";
import {
  buildStepSectionSummaries,
  isStepSectionDirty,
  type StepSection,
} from "@/lib/workflows/workflow-step-section-summary";
import { StepPromptSection } from "@/components/settings/workflow-step-prompt-section";
import { AutomationTab, type WorkflowActionSelection } from "./automation-tab";
import { InspectorHeader } from "./inspector-header";
import { StepSummarySection } from "./step-summary-section";
import { StepAgentFields } from "./step-agent-fields";
import { StepBoardFields } from "./step-board-fields";
import { StepAdvancedFields } from "./step-advanced-fields";

export type WorkflowInspectorProps = {
  step: WorkflowStep;
  savedStep?: WorkflowStep;
  steps: WorkflowStep[];
  readOnly: boolean;
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  onRemove?: () => void;
  onRestoreSource?: () => void;
  focusedAction?: WorkflowActionSelection | null;
  mobile?: boolean;
  onSessionConfigResolutionPendingChange?: (pending: boolean) => void;
  onFocusAction?: (selection: WorkflowActionSelection | null, mode?: "push" | "replace") => void;
};

const SECTION_LABELS: Record<StepSection, string> = {
  agent: "workflows:agentTab",
  instructions: "workflows:stepInstructionsHeading",
  automation: "workflows:automationTab",
  board: "workflows:stepBoardHeading",
  advanced: "workflows:stepAdvancedHeading",
};

export function WorkflowInspector(props: WorkflowInspectorProps) {
  const { t } = useTranslation();
  const { step, savedStep, steps, readOnly, onUpdate } = props;
  const profiles = useAppStore((state) => state.agentProfiles.items);
  const summaries = buildStepSectionSummaries(step, steps, profiles);
  const content = {
    agent: <StepAgentFields {...props} />,
    instructions: (
      <StepPromptSection
        step={step}
        savedStep={savedStep}
        localPrompt={step.prompt ?? ""}
        onPromptChange={(prompt) => onUpdate({ prompt })}
        readOnly={readOnly}
      />
    ),
    automation: (
      <AutomationTab
        step={step}
        steps={steps}
        readOnly={readOnly}
        onUpdate={onUpdate}
        focusedAction={props.focusedAction}
        mobile={props.mobile}
        onFocusAction={props.onFocusAction}
      />
    ),
    board: <StepBoardFields {...props} />,
    advanced: <StepAdvancedFields {...props} />,
  };
  return (
    <section
      className="min-w-0 overflow-hidden rounded-xl border border-border bg-card"
      data-testid="workflow-editor-inspector"
    >
      <InspectorHeader {...props} />
      <div className="divide-y divide-border/60">
        {(Object.keys(SECTION_LABELS) as StepSection[]).map((section) => (
          <StepSummarySection
            key={section}
            section={section}
            label={t(SECTION_LABELS[section])}
            summary={summaries[section].map((part) => t(part.key, part.values)).join(" · ")}
            dirty={isStepSectionDirty(step, savedStep, section)}
            keepMounted={section === "advanced"}
            defaultOpen={section === "automation" && !!props.focusedAction}
          >
            {content[section]}
          </StepSummarySection>
        ))}
      </div>
    </section>
  );
}
