"use client";

import { useTranslation } from "react-i18next";
import { IconArrowLeft } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import type { Workflow, WorkflowStep, Workspace } from "@/lib/types/http";
import { useRouter } from "@/lib/routing/client-router";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { WorkflowPromptSection } from "@/components/settings/workflow-prompt-section";
import {
  buildWorkflowEditorViewModel,
  type WorkflowEditorIssue,
} from "@/lib/workflows/workflow-editor-view-model";
import { WorkflowEditorPipeline } from "./pipeline";
import { WorkflowInspector, type WorkflowInspectorTab } from "./inspector";
import { IssueSummary } from "./issue-summary";
import { MobileWorkflowEditor } from "./mobile-editor";
import { useWorkflowEditorNavigation } from "./workflow-editor-navigation";
import { workflowsPath } from "./workflow-editor-paths";
import { useWorkflowEditorDraft, type WorkflowEditorDraftInput } from "./workflow-editor-draft";

export type WorkflowEditorPageProps = {
  workspace: Workspace;
  workflow: Workflow;
  steps: WorkflowStep[];
  isNewWorkflow?: boolean;
};

export function WorkflowEditorPage({
  workspace,
  workflow,
  steps,
  isNewWorkflow = false,
}: WorkflowEditorPageProps) {
  const { isMobile } = useResponsiveBreakpoint();
  const draft = useWorkflowEditorDraft({
    workspace,
    initialWorkflow: workflow,
    initialSteps: steps,
    isNewWorkflow,
  } satisfies WorkflowEditorDraftInput);
  const navigation = useWorkflowEditorNavigation(draft);

  if (!navigation.selectedStep) {
    return (
      <div className="space-y-4" data-testid="workflow-editor">
        <EditorHeader
          workflow={draft.workflow}
          workspaceId={workspace.id}
          readOnly={draft.readOnly}
          onUpdate={draft.onUpdateWorkflow}
        />
        <WorkflowEditorPipeline
          steps={draft.steps}
          model={draft.model}
          selectedStepId={null}
          onSelectStep={navigation.handleSelectStep}
          onAddStep={draft.onAddStep}
          readOnly={draft.readOnly}
        />
        <EmptyStepNotice onAddStep={draft.onAddStep} readOnly={draft.readOnly} />
      </div>
    );
  }

  const editorProps: EditorSurfaceProps = {
    workspaceId: workspace.id,
    workflow: draft.workflow,
    savedWorkflow: draft.savedWorkflow,
    steps: draft.steps,
    savedSteps: draft.savedSteps,
    model: draft.model,
    selectedStep: navigation.selectedStep,
    savedSelectedStep: draft.savedSteps.find((step) => step.id === navigation.selectedStep?.id),
    selectedStepId: navigation.selectedStep.id,
    activeTab: navigation.activeTab,
    hasStepSelection: navigation.hasStepSelection,
    readOnly: draft.readOnly,
    focusedAction: navigation.focusedAction,
    onSelectStep: navigation.handleSelectStep,
    onAddStep: draft.onAddStep,
    onRemoveStep: draft.onRemoveStep,
    onUpdateStep: draft.onUpdateStep,
    onUpdateWorkflow: draft.onUpdateWorkflow,
    onTabChange: navigation.handleTabChange,
    onFocusAction: navigation.handleFocusAction,
    onBackToJourney: navigation.onBackToJourney,
    onBackToStep: navigation.onBackToStep,
    onSessionConfigResolutionPendingChange: draft.onSessionConfigResolutionPendingChange,
  };
  return isMobile ? (
    <MobileWorkflowEditor {...editorProps} workspaceId={workspace.id} />
  ) : (
    <DesktopWorkflowEditor
      {...editorProps}
      workspaceId={workspace.id}
      onIssue={navigation.handleIssue}
    />
  );
}

export type EditorSurfaceProps = {
  workspaceId: string;
  workflow: Workflow;
  savedWorkflow?: Workflow;
  steps: WorkflowStep[];
  savedSteps: WorkflowStep[];
  model: ReturnType<typeof buildWorkflowEditorViewModel>;
  selectedStep: WorkflowStep;
  savedSelectedStep?: WorkflowStep;
  selectedStepId: string;
  activeTab: WorkflowInspectorTab;
  hasStepSelection: boolean;
  readOnly: boolean;
  focusedAction: {
    trigger: "on_enter" | "on_turn_start" | "on_turn_complete" | "on_exit";
    index: number;
  } | null;
  onSelectStep: (id: string) => void;
  onAddStep: () => void;
  onRemoveStep: (id: string) => void;
  onUpdateStep: (id: string, updates: Partial<WorkflowStep>) => void;
  onUpdateWorkflow: (updates: Partial<Pick<Workflow, "name" | "description" | "prompt">>) => void;
  onTabChange: (tab: WorkflowInspectorTab) => void;
  onFocusAction: (
    focus: {
      trigger: "on_enter" | "on_turn_start" | "on_turn_complete" | "on_exit";
      index: number;
    } | null,
    mode?: "push" | "replace",
  ) => void;
  onBackToJourney: () => void;
  onBackToStep: () => void;
  onSessionConfigResolutionPendingChange: (pending: boolean) => void;
};

function DesktopWorkflowEditor(
  props: EditorSurfaceProps & { onIssue: (issue: WorkflowEditorIssue) => void },
) {
  const { t } = useTranslation();
  return (
    <div className="space-y-5" data-testid="workflow-editor">
      <EditorHeader
        workflow={props.workflow}
        workspaceId={props.workspaceId}
        readOnly={props.readOnly}
        onUpdate={props.onUpdateWorkflow}
      />
      <WorkflowPromptSection
        workflow={props.workflow}
        savedWorkflow={props.savedWorkflow}
        readOnly={props.readOnly}
        onUpdate={(prompt) => props.onUpdateWorkflow({ prompt })}
      />
      <WorkflowEditorPipeline
        steps={props.steps}
        model={props.model}
        selectedStepId={props.selectedStepId}
        onSelectStep={props.onSelectStep}
        onAddStep={props.onAddStep}
        readOnly={props.readOnly}
      />
      <IssueSummary issues={props.model.issues} onIssue={props.onIssue} />
      <WorkflowInspector
        step={props.selectedStep}
        savedStep={props.savedSelectedStep}
        steps={props.steps}
        activeTab={props.activeTab}
        readOnly={props.readOnly}
        focusedAction={props.focusedAction}
        onFocusAction={props.onFocusAction}
        onTabChange={props.onTabChange}
        onUpdate={(updates) => props.onUpdateStep(props.selectedStep.id, updates)}
        onRemove={() => props.onRemoveStep(props.selectedStep.id)}
        onSessionConfigResolutionPendingChange={props.onSessionConfigResolutionPendingChange}
      />
      {props.readOnly && (
        <p className="text-sm text-muted-foreground">{t("workflows:editorReadOnly")}</p>
      )}
    </div>
  );
}

function EditorHeader({
  workflow,
  workspaceId,
  readOnly,
  onUpdate,
}: {
  workflow: Workflow;
  workspaceId: string;
  readOnly: boolean;
  onUpdate: (updates: Partial<Pick<Workflow, "name" | "description" | "prompt">>) => void;
}) {
  const { t } = useTranslation();
  const router = useRouter();
  return (
    <header className="space-y-4" data-testid="workflow-editor-header">
      <Button
        type="button"
        variant="ghost"
        className="min-h-11 cursor-pointer px-2"
        onClick={() => router.push(workflowsPath(workspaceId))}
      >
        <IconArrowLeft className="mr-1.5 h-4 w-4" />
        {t("workflows:returnToWorkflow")}
      </Button>
      <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]">
        <div className="space-y-1.5">
          <Label htmlFor="workflow-editor-name">{t("workflows:workflowName")}</Label>
          <Input
            id="workflow-editor-name"
            data-testid="workflow-editor-name"
            value={workflow.name}
            onChange={(event) => onUpdate({ name: event.target.value })}
            disabled={readOnly}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="workflow-editor-description">{t("workflows:workflowDescription")}</Label>
          <Textarea
            id="workflow-editor-description"
            value={workflow.description ?? ""}
            onChange={(event) => onUpdate({ description: event.target.value })}
            disabled={readOnly}
            className="min-h-10"
          />
        </div>
      </div>
    </header>
  );
}

function EmptyStepNotice({ onAddStep, readOnly }: { onAddStep: () => void; readOnly: boolean }) {
  const { t } = useTranslation();
  return (
    <div className="rounded-lg border border-dashed border-border p-5 text-sm text-muted-foreground">
      <p>{t("workflows:noWorkflowSteps")}</p>
      {!readOnly && (
        <Button
          type="button"
          variant="outline"
          className="mt-3 min-h-11 cursor-pointer"
          onClick={onAddStep}
        >
          {t("workflows:addStep")}
        </Button>
      )}
    </div>
  );
}
