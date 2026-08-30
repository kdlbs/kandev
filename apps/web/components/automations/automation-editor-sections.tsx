import { IconTrash } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Separator } from "@kandev/ui/separator";
import { Switch } from "@kandev/ui/switch";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import type { AutomationTrigger, PlaceholderInfo, TriggerTypeInfo } from "@/lib/types/automation";
import { type CreatedWebhookDetails, type FormState } from "./automation-payload";
import { useAutomationTriggerDrafts } from "./automation-trigger-drafts";
import { ConfigSection } from "./config-section";
import { PromptSection } from "./prompt-section";
import { RequiredFieldLabel } from "./required-field-label";
import { TriggersSection } from "./triggers-section";
import { WebhookCreatedDialog } from "./webhook-created-dialog";
import { useTaskTitleSelectionRestore } from "@/hooks/use-task-title-selection-restore";

type UpdateField = <K extends keyof FormState>(key: K, value: FormState[K]) => void;

const SELECTED_CARD_CLASS_NAME = "border-primary bg-primary/5";
const UNSELECTED_CARD_CLASS_NAME = "border-border hover:bg-muted/30";

export function NameField({
  value,
  isDirty,
  onChange,
}: {
  value: string;
  isDirty: boolean;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div
      className="space-y-2 rounded-lg border bg-card p-4"
      data-settings-dirty={isDirty}
      data-settings-dirty-level="container"
    >
      <RequiredFieldLabel htmlFor="automation-name">
        {t("automations:nameLabel")}
      </RequiredFieldLabel>
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

export function ThenSection({
  form,
  workspaceId,
  placeholders,
  defaultTaskTitle,
  savedForm,
  updateField,
}: {
  form: FormState;
  workspaceId: string;
  placeholders: PlaceholderInfo[];
  defaultTaskTitle: string;
  savedForm: FormState;
  updateField: UpdateField;
}) {
  const { t } = useTranslation();
  const { inputRef, clampChange } = useTaskTitleSelectionRestore(form.taskTitleTemplate);
  const dirtyFields: Array<keyof FormState> = [
    "taskTitleTemplate",
    "prompt",
    "workflowId",
    "taskMode",
    "agentProfileId",
    "executorProfileId",
    "repositorySelections",
  ];
  const isDirty = dirtyFields.some((field) => isAutomationFieldDirty(form, savedForm, field));
  return (
    <div className="space-y-2">
      <div>
        <h3 className="text-base font-medium">{t("automations:thenTitle")}</h3>
        <p className="text-sm text-muted-foreground">{t("automations:thenDescription")}</p>
      </div>
      <div
        className="rounded-lg border bg-card p-4 space-y-4"
        data-settings-dirty={isDirty}
        data-settings-dirty-level="container"
      >
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
        <PromptSection
          value={form.prompt}
          isDirty={isAutomationFieldDirty(form, savedForm, "prompt")}
          onChange={(value) => updateField("prompt", value)}
          placeholders={placeholders}
        />
        <Separator />
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
          }}
          onAgentProfileChange={(value) => updateField("agentProfileId", value)}
          onExecutorProfileChange={(value) => updateField("executorProfileId", value)}
          onRepositoriesChange={(value) => {
            updateField("repositorySelections", value);
            updateField("repositoryMode", value.length > 0 ? "selected" : "none");
          }}
        />
      </div>
    </div>
  );
}

function ContinuationPolicySection({
  form,
  savedForm,
  updateField,
}: {
  form: FormState;
  savedForm: FormState;
  updateField: UpdateField;
}) {
  const { t } = useTranslation();
  const continuationIsDirty = isAutomationFieldDirty(form, savedForm, "continuationPolicy");
  const reusesThread = form.continuationPolicy === "reuse_thread";
  const newTaskDescriptionKey =
    form.taskMode === "normal_task"
      ? "automations:contextBetweenRunsNewTaskNormalDescription"
      : "automations:contextBetweenRunsNewTaskDescription";
  const continuationDescriptionId = "automation-continuation-description";

  return (
    <div className="space-y-2 border-t pt-3">
      <div>
        <h3 id="automation-continuation-heading" className="text-sm font-medium">
          {t("automations:contextBetweenRunsTitle")}
        </h3>
        <p id={continuationDescriptionId} className="text-xs text-muted-foreground">
          {t("automations:contextBetweenRunsDescription")}
        </p>
      </div>
      <RadioGroup
        aria-labelledby="automation-continuation-heading"
        aria-describedby={continuationDescriptionId}
        value={form.continuationPolicy}
        onValueChange={(value) => {
          const policy = value as FormState["continuationPolicy"];
          updateField("continuationPolicy", policy);
          if (policy === "reuse_thread") updateField("maxConcurrentRuns", 1);
        }}
        data-settings-dirty={continuationIsDirty}
        className="gap-2"
      >
        <Label
          htmlFor="automation-continuation-new-task"
          className={`flex min-h-11 w-full cursor-pointer items-start gap-3 rounded-md border p-3 ${
            !reusesThread ? SELECTED_CARD_CLASS_NAME : UNSELECTED_CARD_CLASS_NAME
          }`}
        >
          <RadioGroupItem
            id="automation-continuation-new-task"
            value="new_task"
            aria-describedby="automation-continuation-new-task-description"
            className="mt-0.5"
          />
          <span className="min-w-0 space-y-1">
            <span className="block text-sm font-medium">
              {t("automations:contextBetweenRunsNewTask")}
            </span>
            <span
              id="automation-continuation-new-task-description"
              className="block whitespace-normal break-words text-xs text-muted-foreground"
            >
              {t(newTaskDescriptionKey)}
            </span>
          </span>
        </Label>
        <Label
          htmlFor="automation-continuation-reuse-thread"
          className={`flex min-h-11 w-full cursor-pointer items-start gap-3 rounded-md border p-3 ${
            reusesThread ? SELECTED_CARD_CLASS_NAME : UNSELECTED_CARD_CLASS_NAME
          }`}
        >
          <RadioGroupItem
            id="automation-continuation-reuse-thread"
            value="reuse_thread"
            aria-describedby="automation-continuation-reuse-thread-description"
            className="mt-0.5"
          />
          <span className="min-w-0 space-y-1">
            <span className="block text-sm font-medium">
              {t("automations:contextBetweenRunsReuseThread")}
            </span>
            <span
              id="automation-continuation-reuse-thread-description"
              className="block whitespace-normal break-words text-xs text-muted-foreground"
            >
              {t("automations:contextBetweenRunsReuseThreadDescription")}
            </span>
          </span>
        </Label>
      </RadioGroup>
      {reusesThread && (
        <p className="text-xs text-muted-foreground">
          {t("automations:contextBetweenRunsConcurrencyLock")}
        </p>
      )}
    </div>
  );
}

function TargetModeSection({
  form,
  savedForm,
  updateField,
}: {
  form: FormState;
  savedForm: FormState;
  updateField: UpdateField;
}) {
  const { t } = useTranslation();
  const targetIsDirty = isAutomationFieldDirty(form, savedForm, "taskMode");
  const normalTask = form.taskMode === "normal_task";
  const descriptionId = "automation-task-mode-description";
  return (
    <div className="space-y-2">
      <div>
        <h3 id="automation-task-mode-heading" className="text-sm font-medium">
          {t("automations:taskModeTitle")}
        </h3>
        <p id={descriptionId} className="text-xs text-muted-foreground">
          {t("automations:taskModeDescription")}
        </p>
      </div>
      <RadioGroup
        aria-labelledby="automation-task-mode-heading"
        aria-describedby={descriptionId}
        value={form.taskMode}
        onValueChange={(value) => updateField("taskMode", value as FormState["taskMode"])}
        data-settings-dirty={targetIsDirty}
        className="gap-2"
      >
        <Label
          htmlFor="automation-task-mode-hidden"
          className={`flex min-h-11 w-full cursor-pointer items-start gap-3 rounded-md border p-3 ${
            !normalTask ? SELECTED_CARD_CLASS_NAME : UNSELECTED_CARD_CLASS_NAME
          }`}
        >
          <RadioGroupItem
            id="automation-task-mode-hidden"
            value="automation_run"
            aria-describedby="automation-task-mode-hidden-description"
            className="mt-0.5"
          />
          <span className="min-w-0 space-y-1">
            <span className="block text-sm font-medium">
              {t("automations:taskModeAutomationRun")}
            </span>
            <span
              id="automation-task-mode-hidden-description"
              className="block whitespace-normal break-words text-xs text-muted-foreground"
            >
              {t("automations:taskModeAutomationRunDescription")}
            </span>
          </span>
        </Label>
        <Label
          htmlFor="automation-task-mode-normal"
          className={`flex min-h-11 w-full cursor-pointer items-start gap-3 rounded-md border p-3 ${
            normalTask ? SELECTED_CARD_CLASS_NAME : UNSELECTED_CARD_CLASS_NAME
          }`}
        >
          <RadioGroupItem
            id="automation-task-mode-normal"
            value="normal_task"
            aria-describedby="automation-task-mode-normal-description"
            className="mt-0.5"
          />
          <span className="min-w-0 space-y-1">
            <span className="block text-sm font-medium">{t("automations:taskModeNormalTask")}</span>
            <span
              id="automation-task-mode-normal-description"
              className="block whitespace-normal break-words text-xs text-muted-foreground"
            >
              {t("automations:taskModeNormalTaskDescription")}
            </span>
          </span>
        </Label>
      </RadioGroup>
    </div>
  );
}

export function SettingsSection({
  form,
  savedForm,
  updateField,
}: {
  form: FormState;
  savedForm: FormState;
  updateField: UpdateField;
}) {
  const { t } = useTranslation();
  const enabledIsDirty = isAutomationFieldDirty(form, savedForm, "enabled");
  const maxRunsIsDirty = isAutomationFieldDirty(form, savedForm, "maxConcurrentRuns");
  const continuationIsDirty = isAutomationFieldDirty(form, savedForm, "continuationPolicy");
  const taskModeIsDirty = isAutomationFieldDirty(form, savedForm, "taskMode");
  const reusesThread = form.continuationPolicy === "reuse_thread";
  return (
    <div
      className="space-y-3 rounded-lg border bg-card p-4"
      data-settings-dirty={
        enabledIsDirty || maxRunsIsDirty || continuationIsDirty || taskModeIsDirty
      }
      data-settings-dirty-level="container"
    >
      <Label className="text-xs uppercase tracking-wider text-muted-foreground">
        {t("common:settings")}
      </Label>
      <div className="flex items-center gap-4">
        <div className="flex items-center gap-2">
          <Switch
            checked={form.enabled}
            data-settings-dirty={enabledIsDirty}
            onCheckedChange={(value) => updateField("enabled", value)}
            className="cursor-pointer"
          />
          <Label className="text-sm">{t("automations:enabledLabel")}</Label>
        </div>
        <div className="flex items-center gap-2">
          <Label className="text-sm">{t("automations:maxConcurrentRuns")}</Label>
          <Input
            type="number"
            min={1}
            value={reusesThread ? 1 : form.maxConcurrentRuns}
            data-settings-dirty={maxRunsIsDirty}
            disabled={reusesThread}
            onChange={(event) =>
              updateField("maxConcurrentRuns", Number.parseInt(event.target.value) || 1)
            }
            className="w-20"
          />
        </div>
      </div>
      <TargetModeSection form={form} savedForm={savedForm} updateField={updateField} />
      <ContinuationPolicySection form={form} savedForm={savedForm} updateField={updateField} />
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
