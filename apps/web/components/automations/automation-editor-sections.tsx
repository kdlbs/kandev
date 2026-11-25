import { IconTrash } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Separator } from "@kandev/ui/separator";
import { Switch } from "@kandev/ui/switch";
import type { AutomationTrigger, PlaceholderInfo, TriggerTypeInfo } from "@/lib/types/automation";
import { type CreatedWebhookDetails, type FormState } from "./automation-payload";
import { useAutomationTriggerDrafts } from "./automation-trigger-drafts";
import { ConfigSection } from "./config-section";
import { PlaceholderList, PromptSection } from "./prompt-section";
import { RequiredFieldLabel } from "./required-field-label";
import { TriggersSection } from "./triggers-section";
import { WebhookCreatedDialog } from "./webhook-created-dialog";
import { RetryPolicySection } from "./retry-policy-section";
import { ContinuationPolicySection, FollowUpPromptField } from "./automation-continuation-section";
import { useTaskTitleSelectionRestore } from "@/hooks/use-task-title-selection-restore";

import { TargetModeSection } from "./automation-target-mode-section";

type UpdateField = <K extends keyof FormState>(key: K, value: FormState[K]) => void;
export function NameField({
  value,
  enabled,
  isDirty,
  enabledIsDirty,
  onChange,
  onEnabledChange,
}: {
  value: string;
  enabled: boolean;
  isDirty: boolean;
  enabledIsDirty: boolean;
  onChange: (value: string) => void;
  onEnabledChange: (enabled: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <div
      className="space-y-2 rounded-lg border bg-card p-4"
      data-settings-dirty={isDirty || enabledIsDirty}
      data-settings-dirty-level="container"
    >
      <div className="flex items-center justify-between gap-4">
        <RequiredFieldLabel htmlFor="automation-name">
          {t("automations:nameLabel")}
        </RequiredFieldLabel>
        <Label className="flex min-h-7 cursor-pointer items-center gap-2 text-sm max-md:min-h-11 [@media(pointer:coarse)]:min-h-11">
          {t("automations:enabledLabel")}
          <Switch
            checked={enabled}
            data-settings-dirty={enabledIsDirty}
            onCheckedChange={onEnabledChange}
            className="cursor-pointer"
            data-testid="automation-enabled-switch"
          />
        </Label>
      </div>
      <Input
        id="automation-name"
        data-testid="automation-name-input"
        value={value}
        data-settings-dirty={isDirty}
        onChange={(event) => onChange(event.target.value)}
        placeholder={t("automations:namePlaceholder")}
        aria-describedby={!value.trim() ? "automation-name-help" : undefined}
        aria-invalid={!value.trim() ? true : undefined}
      />
      {!value.trim() && (
        <p id="automation-name-help" className="text-xs text-muted-foreground">
          {t("automations:nameHelp")}
        </p>
      )}
    </div>
  );
}

export function CreatedWebhookDialogHost({
  details,
  onClose,
}: {
  details: CreatedWebhookDetails | null;
  onClose: () => void;
}) {
  if (!details) return null;
  return (
    <WebhookCreatedDialog
      open
      webhookUrl={details.url}
      webhookSecret={details.secret}
      onClose={onClose}
    />
  );
}

type TriggerActionsResult = ReturnType<typeof useAutomationTriggerDrafts>;

export function WhenSection({
  triggerActions,
  triggerTypes,
  currentId,
  workspaceId,
  savedTriggers,
  isDirty,
}: {
  triggerActions: TriggerActionsResult;
  triggerTypes: TriggerTypeInfo[];
  currentId: string | null;
  workspaceId: string;
  savedTriggers: AutomationTrigger[];
  isDirty: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2">
      <div>
        <h3 className="text-base font-medium">{t("automations:whenTitle")}</h3>
        <p className="text-sm text-muted-foreground">{t("automations:whenDescription")}</p>
      </div>
      <div
        className="rounded-lg border bg-card p-4"
        data-settings-dirty={isDirty}
        data-settings-dirty-level="container"
      >
        <TriggersSection
          triggers={triggerActions.allTriggers}
          automationId={currentId}
          workspaceId={workspaceId}
          triggerTypes={triggerTypes}
          savedTriggers={savedTriggers}
          onAddTrigger={triggerActions.handleAdd}
          onUpdateTrigger={triggerActions.handleUpdate}
          onToggleTrigger={triggerActions.handleToggle}
          onDeleteTrigger={triggerActions.handleDelete}
        />
      </div>
    </div>
  );
}

const THEN_DIRTY_FIELDS: Array<keyof FormState> = [
  "continuationPolicy",
  "prContinuationStepId",
  "prMergedAction",
  "prClosedAction",
  "taskTitleTemplate",
  "prompt",
  "followUpPrompt",
  "workflowId",
  "taskMode",
  "agentProfileId",
  "executorProfileId",
  "repositorySelections",
  "managedDestination",
];

function TaskTitleField({
  form,
  savedForm,
  defaultTaskTitle,
  updateField,
}: {
  form: FormState;
  savedForm: FormState;
  defaultTaskTitle: string;
  updateField: UpdateField;
}) {
  const { t } = useTranslation();
  const { inputRef, clampChange } = useTaskTitleSelectionRestore(form.taskTitleTemplate);
  return (
    <div className="space-y-1.5">
      <Label className="text-xs">{t("automations:taskTitleLabel")}</Label>
      <Input
        ref={inputRef}
        value={form.taskTitleTemplate}
        data-settings-dirty={isAutomationFieldDirty(form, savedForm, "taskTitleTemplate")}
        onChange={(event) => updateField("taskTitleTemplate", clampChange(event))}
        // defaultTaskTitle is the backend trigger type's own template — a
        // persisted value, not copy. The fallback is the example hint.
        placeholder={defaultTaskTitle || t("automations:taskTitlePlaceholder")}
      />
      <p className="text-xs text-muted-foreground">{t("automations:taskTitleHelp")}</p>
    </div>
  );
}

function ThenConfigSection({
  form,
  savedForm,
  workspaceId,
  updateField,
}: {
  form: FormState;
  savedForm: FormState;
  workspaceId: string;
  updateField: UpdateField;
}) {
  return (
    <ConfigSection
      workspaceId={workspaceId}
      workflowId={form.workflowId}
      agentProfileId={form.agentProfileId}
      executorProfileId={form.executorProfileId}
      taskMode={form.taskMode}
      repositorySelections={form.repositorySelections}
      dirtyFields={{
        workflowId: isAutomationFieldDirty(form, savedForm, "workflowId"),
        agentProfileId: isAutomationFieldDirty(form, savedForm, "agentProfileId"),
        executorProfileId: isAutomationFieldDirty(form, savedForm, "executorProfileId"),
        repositorySelections: isAutomationFieldDirty(form, savedForm, "repositorySelections"),
      }}
      onWorkflowChange={(value) => {
        updateField("workflowId", value);
        updateField("workflowStepId", "");
        updateField("prContinuationStepId", "");
      }}
      onAgentProfileChange={(value) => updateField("agentProfileId", value)}
      onExecutorProfileChange={(value) => updateField("executorProfileId", value)}
      onRepositoriesChange={(value) => {
        updateField("repositorySelections", value);
        updateField("repositoryMode", value.length > 0 ? "selected" : "none");
      }}
    />
  );
}

export function ThenSection({
  form,
  workspaceId,
  placeholders,
  defaultTaskTitle,
  savedForm,
  updateField,
  hasPRTrigger,
}: {
  form: FormState;
  workspaceId: string;
  placeholders: PlaceholderInfo[];
  defaultTaskTitle: string;
  savedForm: FormState;
  updateField: UpdateField;
  hasPRTrigger: boolean;
}) {
  const { t } = useTranslation();
  const managed = form.taskMode === "managed_conversation";
  const isDirty = THEN_DIRTY_FIELDS.some((field) => isAutomationFieldDirty(form, savedForm, field));
  return (
    <div className="space-y-2">
      <div>
        <h3 className="text-base font-medium">{t("automations:thenTitle")}</h3>
        <p className="text-sm text-muted-foreground">
          {t(
            managed
              ? "automations:thenManagedConversationDescription"
              : "automations:thenDescription",
          )}
        </p>
      </div>
      <div
        className="rounded-lg border bg-card p-4 space-y-4"
        data-settings-dirty={isDirty}
        data-settings-dirty-level="container"
      >
        <TargetModeSection
          form={form}
          savedForm={savedForm}
          workspaceId={workspaceId}
          updateField={updateField}
        />
        {managed ? null : (
          <ContinuationPolicySection
            form={form}
            savedForm={savedForm}
            updateField={updateField}
            hasPRTrigger={hasPRTrigger}
          />
        )}
        <Separator />
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
          isDirty={isAutomationFieldDirty(form, savedForm, "prompt")}
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
        {managed ? null : (
          <>
            <Separator />
            <ThenConfigSection
              form={form}
              savedForm={savedForm}
              workspaceId={workspaceId}
              updateField={updateField}
            />
          </>
        )}
      </div>
    </div>
  );
}

export function LimitsSection({
  form,
  savedForm,
  updateField,
}: {
  form: FormState;
  savedForm: FormState;
  updateField: UpdateField;
}) {
  const { t } = useTranslation();
  const maxRunsIsDirty = isAutomationFieldDirty(form, savedForm, "maxConcurrentRuns");
  const retryPolicyIsDirty = isAutomationFieldDirty(form, savedForm, "retryPolicy");
  const reusesThread = form.continuationPolicy === "reuse_thread";
  return (
    <div className="space-y-2">
      <h3 className="text-base font-medium">{t("automations:limitsTitle")}</h3>
      <div
        className="space-y-4 rounded-lg border bg-card p-4"
        data-settings-dirty={maxRunsIsDirty || retryPolicyIsDirty}
        data-settings-dirty-level="container"
      >
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
            data-settings-dirty={maxRunsIsDirty}
            disabled={reusesThread}
            aria-describedby={reusesThread ? "automation-max-concurrent-runs-lock" : undefined}
            onChange={(event) =>
              updateField("maxConcurrentRuns", Number.parseInt(event.target.value) || 1)
            }
            className="w-20"
          />
        </div>
        {form.taskMode === "managed_conversation" ? null : (
          <RetryPolicySection
            policy={form.retryPolicy}
            savedPolicy={savedForm.retryPolicy}
            updateField={updateField}
          />
        )}
      </div>
    </div>
  );
}

export function EditorFooter({
  saving,
  isNew,
  onDelete,
}: {
  saving: boolean;
  isNew: boolean;
  onDelete: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-3 pt-4">
      {!isNew && (
        <Button
          data-testid="automation-delete-button"
          variant="destructive"
          className="cursor-pointer"
          onClick={onDelete}
          disabled={saving}
        >
          <IconTrash className="h-4 w-4 mr-1" />
          {t("automations:delete")}
        </Button>
      )}
    </div>
  );
}

export function isAutomationFieldDirty<K extends keyof FormState>(
  form: FormState,
  savedForm: FormState,
  field: K,
): boolean {
  return JSON.stringify(form[field]) !== JSON.stringify(savedForm[field]);
}
