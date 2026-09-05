"use client";

import { useTranslation } from "react-i18next";
import { IconArrowLeft } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { useRouter } from "@/lib/routing/client-router";
import type { Workflow, WorkflowStep } from "@/lib/types/http";
import { WorkflowPromptSection } from "@/components/settings/workflow-prompt-section";
import { WorkflowInspector } from "./inspector";
import { WorkflowActionEditor } from "./action-editor";
import type { WorkflowActionRecord } from "@/lib/workflows/workflow-action-catalog";
import type { WorkflowActionSelection } from "./automation-tab";
import {
  moveWorkflowAction,
  removeWorkflowAction,
  updateWorkflowAction,
} from "@/components/settings/workflow-step-mutations";
import type { EditorSurfaceProps } from "./workflow-editor-page";
import { IssueSummary } from "./issue-summary";
import { WorkflowEditorPipeline } from "./pipeline";
import { workflowsPath } from "./workflow-editor-paths";

type MobileWorkflowEditorProps = EditorSurfaceProps & { workspaceId: string };

export function MobileWorkflowEditor(props: MobileWorkflowEditorProps) {
  const router = useRouter();
  const selectStep = (stepId: string) => {
    props.onSelectStep(stepId);
  };
  const handleIssue = (issue: (typeof props.model.issues)[number]) => {
    if (issue.target === "workflow") {
      document.getElementById("workflow-editor-mobile-name")?.focus();
      return;
    }
    selectStep(issue.stepId);
    if (issue.trigger) props.onTabChange("automation");
    if (issue.trigger && issue.actionIndex !== undefined) {
      props.onFocusAction({ trigger: issue.trigger, index: issue.actionIndex });
    }
  };
  const screen = mobileEditorScreen(props);
  let editorContent: React.ReactNode;
  if (screen === "journey") {
    editorContent = <MobileJourney {...props} onSelectStep={selectStep} onIssue={handleIssue} />;
  } else if (screen === "step") {
    editorContent = (
      <MobileStepScreen
        {...props}
        onBack={props.onBackToJourney}
        onRemove={() => {
          props.onRemoveStep(props.selectedStep.id);
          props.onBackToJourney();
        }}
      />
    );
  } else {
    editorContent = <MobileActionScreen {...props} onBack={props.onBackToStep} />;
  }
  return (
    <div
      className="min-w-0 max-w-full space-y-4 overflow-x-hidden"
      data-testid="workflow-editor-mobile"
    >
      <MobileWorkflowHeader
        workflow={props.workflow}
        readOnly={props.readOnly}
        onUpdate={props.onUpdateWorkflow}
        onBack={() => router.push(workflowsPath(props.workspaceId))}
      />
      {editorContent}
    </div>
  );
}

function mobileEditorScreen(
  props: Pick<MobileWorkflowEditorProps, "focusedAction" | "hasStepSelection">,
) {
  if (props.focusedAction) return "action" as const;
  if (props.hasStepSelection) return "step" as const;
  return "journey" as const;
}

function MobileWorkflowHeader({
  workflow,
  readOnly,
  onUpdate,
  onBack,
}: {
  workflow: Workflow;
  readOnly: boolean;
  onUpdate: EditorSurfaceProps["onUpdateWorkflow"];
  onBack: () => void;
}) {
  const { t } = useTranslation();
  return (
    <header className="space-y-3" data-testid="workflow-editor-mobile-header">
      <Button
        type="button"
        variant="ghost"
        className="min-h-11 cursor-pointer px-2"
        onClick={onBack}
      >
        <IconArrowLeft className="mr-1.5 h-4 w-4" />
        {t("workflows:returnToWorkflow")}
      </Button>
      <div className="space-y-3">
        <div className="space-y-1.5">
          <Label htmlFor="workflow-editor-mobile-name">{t("workflows:workflowName")}</Label>
          <Input
            id="workflow-editor-mobile-name"
            value={workflow.name}
            onChange={(event) => onUpdate({ name: event.target.value })}
            disabled={readOnly}
          />
        </div>
        <WorkflowPromptSection
          workflow={workflow}
          readOnly={readOnly}
          onUpdate={(prompt) => onUpdate({ prompt })}
        />
      </div>
    </header>
  );
}

function MobileJourney(
  props: MobileWorkflowEditorProps & {
    onSelectStep: (stepId: string) => void;
    onIssue: (issue: (typeof props.model.issues)[number]) => void;
  },
) {
  return (
    <div
      className="min-h-[calc(100dvh-10rem)] space-y-5 overflow-y-auto pb-[max(6rem,env(safe-area-inset-bottom))]"
      data-testid="workflow-editor-mobile-journey-screen"
    >
      <WorkflowEditorPipeline
        steps={props.steps}
        model={props.model}
        selectedStepId={props.selectedStepId}
        onSelectStep={props.onSelectStep}
        onAddStep={props.onAddStep}
        readOnly={props.readOnly}
        mobile
      />
      <IssueSummary issues={props.model.issues} onIssue={props.onIssue} />
    </div>
  );
}

function MobileStepScreen(
  props: MobileWorkflowEditorProps & {
    onBack: () => void;
    onRemove: () => void;
  },
) {
  const { t } = useTranslation();
  return (
    <section
      className="min-h-[calc(100dvh-10rem)] space-y-4 overflow-y-auto pb-[max(6rem,env(safe-area-inset-bottom))]"
      data-testid="workflow-editor-mobile-step-screen"
    >
      <Button
        type="button"
        variant="ghost"
        className="min-h-11 cursor-pointer px-2"
        onClick={props.onBack}
      >
        <IconArrowLeft className="mr-1.5 h-4 w-4" />
        {t("workflows:backToJourney")}
      </Button>
      <WorkflowInspector
        step={props.selectedStep}
        savedStep={props.savedSelectedStep}
        steps={props.steps}
        activeTab={props.activeTab}
        readOnly={props.readOnly}
        focusedAction={props.focusedAction}
        mobile
        onFocusAction={props.onFocusAction}
        onTabChange={props.onTabChange}
        onUpdate={(updates) => props.onUpdateStep(props.selectedStep.id, updates)}
        onRemove={props.onRemove}
        onSessionConfigResolutionPendingChange={props.onSessionConfigResolutionPendingChange}
      />
    </section>
  );
}

function MobileActionScreen(props: MobileWorkflowEditorProps & { onBack: () => void }) {
  const { t } = useTranslation();
  const selection = props.focusedAction;
  const actions = selection ? actionsFor(props.selectedStep, selection.trigger) : [];
  const action = selection ? actions[selection.index] : undefined;
  if (!selection || !action) return null;

  const updateAction = (updates: Partial<WorkflowActionRecord>) => {
    const next = updateWorkflowAction(
      props.selectedStep,
      selection.trigger,
      selection.index,
      updates,
    );
    props.onUpdateStep(props.selectedStep.id, { events: next.events });
  };
  const moveAction = (direction: -1 | 1) => {
    const nextIndex = selection.index + direction;
    const next = moveWorkflowAction(
      props.selectedStep,
      selection.trigger,
      selection.index,
      nextIndex,
    );
    props.onUpdateStep(props.selectedStep.id, { events: next.events });
    props.onFocusAction({ ...selection, index: nextIndex }, "replace");
  };
  const removeAction = () => {
    const next = removeWorkflowAction(props.selectedStep, selection.trigger, selection.index);
    props.onUpdateStep(props.selectedStep.id, { events: next.events });
    props.onBack();
  };

  return (
    <section
      className="min-h-[calc(100dvh-10rem)] space-y-4 overflow-y-auto pb-[max(6rem,env(safe-area-inset-bottom))]"
      data-testid="workflow-editor-mobile-action-screen"
    >
      <Button
        type="button"
        variant="ghost"
        className="min-h-11 cursor-pointer px-2"
        onClick={props.onBack}
      >
        <IconArrowLeft className="mr-1.5 h-4 w-4" />
        {t("workflows:backToStep")}
      </Button>
      <WorkflowActionEditor
        action={action}
        actionIndex={selection.index}
        actionCount={actions.length}
        trigger={selection.trigger}
        steps={props.steps}
        readOnly={props.readOnly}
        onChange={updateAction}
        onMove={moveAction}
        onRemove={removeAction}
      />
    </section>
  );
}

function actionsFor(
  step: WorkflowStep,
  trigger: WorkflowActionSelection["trigger"],
): WorkflowActionRecord[] {
  return (step.events?.[trigger] ?? []) as unknown as WorkflowActionRecord[];
}
