import { IconTrash } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import type { AutomationTrigger, TriggerTypeInfo } from "@/lib/types/automation";
import { type CreatedWebhookDetails, type FormState } from "./automation-payload";
import { useAutomationTriggerDrafts } from "./automation-trigger-drafts";
import { EditorSection } from "./editor-section";
import { RequiredFieldLabel } from "./required-field-label";
import { TriggersSection } from "./triggers-section";
import { WebhookCreatedDialog } from "./webhook-created-dialog";

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
    <EditorSection
      title={t("automations:whenTitle")}
      description={t("automations:whenDescription")}
      dirty={isDirty}
      testId="automation-when-section"
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
    </EditorSection>
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
