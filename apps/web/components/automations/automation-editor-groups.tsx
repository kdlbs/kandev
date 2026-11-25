"use client";

import { useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Separator } from "@kandev/ui/separator";
import type { PlaceholderInfo } from "@/lib/types/automation";
import { useTaskTitleSelectionRestore } from "@/hooks/use-task-title-selection-restore";
import type { FormState } from "./automation-payload";
import { ConfigSection, WorkflowField } from "./config-section";
import { PlaceholderList, PromptSection } from "./prompt-section";
import { RetryPolicySection } from "./retry-policy-section";
import { ContinuationPolicySection, FollowUpPromptField } from "./automation-continuation-section";
import { TargetModeSection } from "./automation-target-mode-section";
import {
  AgentToolsDisclosure,
  isMCPSelectionDirty,
  isPluginToolsDirty,
  type AgentToolsDisclosureProps,
} from "./automation-agent-tools-section";
import { isAutomationFieldDirty } from "./automation-editor-sections";
import { EditorSection } from "./editor-section";

type UpdateField = <K extends keyof FormState>(key: K, value: FormState[K]) => void;

type GroupProps = {
  form: FormState;
  savedForm: FormState;
  updateField: UpdateField;
};

function isDirty(form: FormState, savedForm: FormState, fields: Array<keyof FormState>): boolean {
  return fields.some((field) => isAutomationFieldDirty(form, savedForm, field));
}

/** Where runs go: the run destination, then the workflow or conversation it needs. */
export function DestinationSection({
  form,
  savedForm,
  workspaceId,
  updateField,
}: GroupProps & { workspaceId: string }) {
  const { t } = useTranslation();
  return (
    <EditorSection
      title={t("automations:destinationTitle")}
      description={t("automations:destinationDescription")}
      dirty={isDirty(form, savedForm, ["taskMode", "managedDestination", "workflowId"])}
      testId="automation-destination-section"
    >
      <TargetModeSection
        form={form}
        savedForm={savedForm}
        workspaceId={workspaceId}
        updateField={updateField}
      />
      {form.taskMode === "managed_conversation" ? null : (
        <WorkflowField
          workspaceId={workspaceId}
          workflowId={form.workflowId}
          taskMode={form.taskMode}
          isDirty={isDirty(form, savedForm, ["workflowId"])}
          onWorkflowChange={(value) => {
            if (value === form.workflowId) return;
            updateField("workflowId", value);
            updateField("workflowStepId", "");
            updateField("prContinuationStepId", "");
          }}
        />
      )}
    </EditorSection>
  );
}

function MaxConcurrentRunsField({ form, savedForm, updateField }: GroupProps) {
  const { t } = useTranslation();
  const dirty = isDirty(form, savedForm, ["maxConcurrentRuns"]);
  const reusesThread = form.continuationPolicy === "reuse_thread";
  return (
    <div className="flex flex-wrap items-center justify-between gap-2">
      <div className="min-w-0">
        <Label htmlFor="automation-max-concurrent-runs" className="text-sm">
          {t("automations:maxConcurrentRuns")}
        </Label>
        {reusesThread && (
          <p id="automation-max-concurrent-runs-lock" className="text-xs text-muted-foreground">
            {t("automations:contextBetweenRunsConcurrencyLock")}
          </p>
        )}
      </div>
      <Input
        id="automation-max-concurrent-runs"
        type="number"
        min={1}
        value={reusesThread ? 1 : form.maxConcurrentRuns}
        data-settings-dirty={dirty}
        disabled={reusesThread}
        aria-describedby={reusesThread ? "automation-max-concurrent-runs-lock" : undefined}
        onChange={(event) =>
          updateField("maxConcurrentRuns", Number.parseInt(event.target.value) || 1)
        }
        className="w-20"
      />
    </div>
  );
}

/** Repeat runs: whether runs share a task, and how many may run at once. */
export function RepeatRunsSection({
  form,
  savedForm,
  updateField,
  hasPRTrigger,
}: GroupProps & { hasPRTrigger: boolean }) {
  const { t } = useTranslation();
  const dirty = isDirty(form, savedForm, [
    "continuationPolicy",
    "prContinuationStepId",
    "prMergedAction",
    "prClosedAction",
    "maxConcurrentRuns",
  ]);
  return (
    <EditorSection
      title={t("automations:repeatRunsTitle")}
      description={t("automations:repeatRunsDescription")}
      dirty={dirty}
      testId="automation-repeat-runs-section"
    >
      {form.taskMode === "managed_conversation" ? null : (
        <ContinuationPolicySection
          form={form}
          savedForm={savedForm}
          updateField={updateField}
          hasPRTrigger={hasPRTrigger}
        />
      )}
      <MaxConcurrentRunsField form={form} savedForm={savedForm} updateField={updateField} />
    </EditorSection>
  );
}

function TaskTitleField({
  form,
  savedForm,
  defaultTaskTitle,
  updateField,
}: GroupProps & { defaultTaskTitle: string }) {
  const { t } = useTranslation();
  const { inputRef, clampChange } = useTaskTitleSelectionRestore(form.taskTitleTemplate);
  return (
    <div className="space-y-1.5">
      <Label htmlFor="automation-task-title" className="text-xs">
        {t("automations:taskTitleLabel")}
      </Label>
      <Input
        id="automation-task-title"
        aria-describedby="automation-task-title-help"
        ref={inputRef}
        value={form.taskTitleTemplate}
        data-settings-dirty={isDirty(form, savedForm, ["taskTitleTemplate"])}
        onChange={(event) => updateField("taskTitleTemplate", clampChange(event))}
        // defaultTaskTitle is the backend trigger type's own template — a
        // persisted value, not copy. The fallback is the example hint.
        placeholder={defaultTaskTitle || t("automations:taskTitlePlaceholder")}
      />
      <p id="automation-task-title-help" className="text-xs text-muted-foreground">
        {t("automations:taskTitleHelp")}
      </p>
    </div>
  );
}

/** Instructions: every prompt text field, below the trigger and policy that shape it. */
export function InstructionsSection({
  form,
  savedForm,
  updateField,
  placeholders,
  defaultTaskTitle,
}: GroupProps & { placeholders: PlaceholderInfo[]; defaultTaskTitle: string }) {
  const { t } = useTranslation();
  const managed = form.taskMode === "managed_conversation";
  return (
    <EditorSection
      title={t("automations:instructionsTitle")}
      description={t(
        managed
          ? "automations:thenManagedConversationDescription"
          : "automations:instructionsDescription",
      )}
      dirty={isDirty(form, savedForm, ["taskTitleTemplate", "prompt", "followUpPrompt"])}
      testId="automation-instructions-section"
    >
      {managed ? null : (
        <TaskTitleField
          form={form}
          savedForm={savedForm}
          defaultTaskTitle={defaultTaskTitle}
          updateField={updateField}
        />
      )}
      <PromptSection
        value={form.prompt}
        isDirty={isDirty(form, savedForm, ["prompt"])}
        onChange={(value) => updateField("prompt", value)}
        placeholders={placeholders}
      />
      <FollowUpPromptField
        form={form}
        savedForm={savedForm}
        updateField={updateField}
        placeholders={placeholders}
      />
      <PlaceholderList placeholders={placeholders} />
    </EditorSection>
  );
}

/** Agent: who does the work, with which repositories, and with which tools. */
export function AgentSection(props: AgentToolsDisclosureProps) {
  const { form, savedForm, workspaceId, updateField } = props;
  const { t } = useTranslation();
  // A managed conversation runs in the plugin's own session.
  if (form.taskMode === "managed_conversation") return null;
  return (
    <EditorSection
      title={t("automations:agentTitle")}
      description={t("automations:agentDescription")}
      dirty={
        isDirty(form, savedForm, ["agentProfileId", "executorProfileId", "repositorySelections"]) ||
        isMCPSelectionDirty(form, savedForm) ||
        (form.taskMode === "automation_run" && isPluginToolsDirty(form, savedForm))
      }
      testId="automation-agent-section"
    >
      <ConfigSection
        workspaceId={workspaceId}
        agentProfileId={form.agentProfileId}
        executorProfileId={form.executorProfileId}
        repositorySelections={form.repositorySelections}
        dirtyFields={{
          agentProfileId: isDirty(form, savedForm, ["agentProfileId"]),
          executorProfileId: isDirty(form, savedForm, ["executorProfileId"]),
          repositorySelections: isDirty(form, savedForm, ["repositorySelections"]),
        }}
        onAgentProfileChange={(value) => updateField("agentProfileId", value)}
        onExecutorProfileChange={(value) => updateField("executorProfileId", value)}
        onRepositoriesChange={(value) => {
          updateField("repositorySelections", value);
          updateField("repositoryMode", value.length > 0 ? "selected" : "none");
        }}
      />
      <Separator />
      <AgentToolsDisclosure {...props} />
    </EditorSection>
  );
}

/** If a run fails: automatic retries. A managed conversation does not retry. */
export function FailureSection({ form, savedForm, updateField }: GroupProps) {
  const { t } = useTranslation();
  if (form.taskMode === "managed_conversation") return null;
  return (
    <EditorSection
      title={t("automations:failureTitle")}
      dirty={isDirty(form, savedForm, ["retryPolicy"])}
      testId="automation-failure-section"
    >
      <RetryPolicySection
        policy={form.retryPolicy}
        savedPolicy={savedForm.retryPolicy}
        updateField={updateField}
      />
    </EditorSection>
  );
}
