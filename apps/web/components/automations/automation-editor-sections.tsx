import { IconTrash } from "@tabler/icons-react";
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
  return (
    <div
      className="space-y-2 rounded-lg border bg-card p-4"
      data-settings-dirty={isDirty}
      data-settings-dirty-level="container"
    >
      <RequiredFieldLabel htmlFor="automation-name">Name</RequiredFieldLabel>
      <Input
        id="automation-name"
        data-testid="automation-name-input"
        value={value}
        data-settings-dirty={isDirty}
        onChange={(event) => onChange(event.target.value)}
        placeholder="Automation name"
        aria-describedby={!value.trim() ? "automation-name-help" : undefined}
        aria-invalid={!value.trim() ? true : undefined}
      />
      {!value.trim() && (
        <p id="automation-name-help" className="text-xs text-muted-foreground">
          Enter an automation name to enable saving.
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
  return (
    <div className="space-y-2">
      <div>
        <h3 className="text-base font-medium">When</h3>
        <p className="text-sm text-muted-foreground">What causes this automation to run</p>
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
  const dirtyFields: Array<keyof FormState> = [
    "taskTitleTemplate",
    "prompt",
    "workflowId",
    "workflowStepId",
    "agentProfileId",
    "executorProfileId",
    "repositorySelection",
    "executionMode",
  ];
  const isDirty = dirtyFields.some((field) => isAutomationFieldDirty(form, savedForm, field));
  return (
    <div className="space-y-2">
      <div>
        <h3 className="text-base font-medium">Then</h3>
        <p className="text-sm text-muted-foreground">
          A new task will be created each time this automation triggers
        </p>
      </div>
      <div
        className="rounded-lg border bg-card p-4 space-y-4"
        data-settings-dirty={isDirty}
        data-settings-dirty-level="container"
      >
        <div className="space-y-1.5">
          <Label className="text-xs">Task title</Label>
          <Input
            value={form.taskTitleTemplate}
            data-settings-dirty={isAutomationFieldDirty(form, savedForm, "taskTitleTemplate")}
            onChange={(event) => updateField("taskTitleTemplate", event.target.value)}
            placeholder={defaultTaskTitle || "[Auto] automation name"}
          />
          <p className="text-xs text-muted-foreground">
            Leave empty to use the default. Supports placeholders.
          </p>
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
          repositorySelection={form.repositorySelection}
          executionMode={form.executionMode}
          conditionType={conditionType}
          dirtyFields={{
            executionMode: isAutomationFieldDirty(form, savedForm, "executionMode"),
            workflowId: isAutomationFieldDirty(form, savedForm, "workflowId"),
            workflowStepId: isAutomationFieldDirty(form, savedForm, "workflowStepId"),
            agentProfileId: isAutomationFieldDirty(form, savedForm, "agentProfileId"),
            executorProfileId: isAutomationFieldDirty(form, savedForm, "executorProfileId"),
            repositorySelection: isAutomationFieldDirty(form, savedForm, "repositorySelection"),
          }}
          onWorkflowChange={(value) => {
            updateField("workflowId", value);
            updateField("workflowStepId", "");
          }}
          onStepChange={(value) => updateField("workflowStepId", value)}
          onAgentProfileChange={(value) => updateField("agentProfileId", value)}
          onExecutorProfileChange={(value) => updateField("executorProfileId", value)}
          onRepositoryChange={(value) => updateField("repositorySelection", value)}
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
  const enabledIsDirty = isAutomationFieldDirty(form, savedForm, "enabled");
  const maxRunsIsDirty = isAutomationFieldDirty(form, savedForm, "maxConcurrentRuns");
  return (
    <div
      className="space-y-3 rounded-lg border bg-card p-4"
      data-settings-dirty={enabledIsDirty || maxRunsIsDirty}
      data-settings-dirty-level="container"
    >
      <Label className="text-xs uppercase tracking-wider text-muted-foreground">Settings</Label>
      <div className="flex items-center gap-4">
        <div className="flex items-center gap-2">
          <Switch
            checked={form.enabled}
            data-settings-dirty={enabledIsDirty}
            onCheckedChange={(value) => updateField("enabled", value)}
            className="cursor-pointer"
          />
          <Label className="text-sm">Enabled</Label>
        </div>
        <div className="flex items-center gap-2">
          <Label className="text-sm">Max concurrent runs</Label>
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
          Delete
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
