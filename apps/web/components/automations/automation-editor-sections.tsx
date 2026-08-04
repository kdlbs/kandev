import { IconTrash } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Separator } from "@kandev/ui/separator";
import { Switch } from "@kandev/ui/switch";
import type {
  AutomationTrigger,
  PlaceholderInfo,
  TriggerType,
  TriggerTypeInfo,
} from "@/lib/types/automation";
import type { CreatedWebhookDetails, FormState } from "./automation-payload";
import { useAutomationTriggerDrafts } from "./automation-trigger-drafts";
import { ConfigSection } from "./config-section";
import { PromptSection } from "./prompt-section";
import { RequiredFieldLabel } from "./required-field-label";
import { TriggersSection } from "./triggers-section";
import { WebhookCreatedDialog } from "./webhook-created-dialog";
import { clampTaskTitleInput } from "@/lib/task-title";

type UpdateField = <K extends keyof FormState>(key: K, value: FormState[K]) => void;

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
  conditionType,
  savedForm,
  updateField,
}: {
  form: FormState;
  workspaceId: string;
  placeholders: PlaceholderInfo[];
  defaultTaskTitle: string;
  conditionType: TriggerType | null;
  savedForm: FormState;
  updateField: UpdateField;
}) {
  const { t } = useTranslation();
  const dirtyFields: Array<keyof FormState> = [
    "taskTitleTemplate",
    "prompt",
    "workflowId",
    "workflowStepId",
    "agentProfileId",
    "executorProfileId",
    "repositorySelections",
    "executionMode",
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
            value={form.taskTitleTemplate}
            data-settings-dirty={isAutomationFieldDirty(form, savedForm, "taskTitleTemplate")}
            onChange={(event) =>
              updateField("taskTitleTemplate", clampTaskTitleInput(event.target.value))
            }
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
          workflowStepId={form.workflowStepId}
          agentProfileId={form.agentProfileId}
          executorProfileId={form.executorProfileId}
          repositorySelections={form.repositorySelections}
          executionMode={form.executionMode}
          conditionType={conditionType}
          dirtyFields={{
            executionMode: isAutomationFieldDirty(form, savedForm, "executionMode"),
            workflowId: isAutomationFieldDirty(form, savedForm, "workflowId"),
            workflowStepId: isAutomationFieldDirty(form, savedForm, "workflowStepId"),
            agentProfileId: isAutomationFieldDirty(form, savedForm, "agentProfileId"),
            executorProfileId: isAutomationFieldDirty(form, savedForm, "executorProfileId"),
            repositorySelections: isAutomationFieldDirty(form, savedForm, "repositorySelections"),
          }}
          onWorkflowChange={(value) => {
            updateField("workflowId", value);
            updateField("workflowStepId", "");
          }}
          onStepChange={(value) => updateField("workflowStepId", value)}
          onAgentProfileChange={(value) => updateField("agentProfileId", value)}
          onExecutorProfileChange={(value) => updateField("executorProfileId", value)}
          onRepositoriesChange={(value) => updateField("repositorySelections", value)}
          onExecutionModeChange={(value) => updateField("executionMode", value)}
        />
      </div>
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
  return (
    <div
      className="space-y-3 rounded-lg border bg-card p-4"
      data-settings-dirty={enabledIsDirty || maxRunsIsDirty}
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
            value={form.maxConcurrentRuns}
            data-settings-dirty={maxRunsIsDirty}
            onChange={(event) =>
              updateField("maxConcurrentRuns", Number.parseInt(event.target.value) || 1)
            }
            className="w-20"
          />
        </div>
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
