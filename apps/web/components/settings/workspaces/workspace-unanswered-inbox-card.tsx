"use client";

import { useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { SettingsGroup } from "@/components/settings/settings-group";

type Props = {
  canManage: boolean;
  enabled: boolean;
  retentionDays: string;
  retentionValid: boolean;
  enabledIsDirty: boolean;
  retentionIsDirty: boolean;
  onEnabledChange: (enabled: boolean) => void;
  onRetentionChange: (value: string) => void;
};

export function WorkspaceUnansweredInboxCard({
  canManage,
  enabled,
  retentionDays,
  retentionValid,
  enabledIsDirty,
  retentionIsDirty,
  onEnabledChange,
  onRetentionChange,
}: Props) {
  const { t } = useTranslation();

  return (
    <SettingsGroup
      title={t("workspaces:unansweredInboxTitle")}
      description={t("workspaces:unansweredInboxDescription")}
      isDirty={enabledIsDirty || retentionIsDirty}
      contentClassName="space-y-4 divide-y-0"
      data-testid="workspace-unanswered-inbox-card"
    >
      <div className="flex min-h-11 items-center justify-between gap-4">
        <div className="space-y-1">
          <Label htmlFor="workspace-unanswered-inbox" className="cursor-pointer">
            {t("workspaces:unansweredInboxEnabled")}
          </Label>
          <p className="text-xs text-muted-foreground">
            {t("workspaces:unansweredInboxEnabledDescription")}
          </p>
        </div>
        <Switch
          id="workspace-unanswered-inbox"
          checked={enabled}
          onCheckedChange={onEnabledChange}
          disabled={!canManage}
          data-settings-dirty={enabledIsDirty}
          data-testid="workspace-unanswered-inbox-switch"
          className="[@media(pointer:coarse)]:after:-inset-y-3"
        />
      </div>
      <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_12rem] sm:items-center sm:gap-4">
        <div className="space-y-1">
          <Label htmlFor="workspace-unanswered-retention">
            {t("workspaces:unansweredRetentionDays")}
          </Label>
          <p className="text-xs text-muted-foreground">
            {t("workspaces:unansweredRetentionDescription")}
          </p>
        </div>
        <Input
          id="workspace-unanswered-retention"
          type="number"
          min={1}
          max={90}
          step={1}
          value={retentionDays}
          disabled={!canManage || !enabled}
          aria-invalid={!retentionValid}
          aria-describedby={!retentionValid ? "workspace-unanswered-retention-error" : undefined}
          data-settings-dirty={retentionIsDirty}
          data-testid="workspace-unanswered-retention-input"
          controlSize="compact"
          className="text-base sm:text-sm md:text-xs/relaxed [@media(pointer:coarse)]:min-h-11"
          onChange={(event) => onRetentionChange(event.target.value)}
        />
        {!retentionValid && (
          <p
            id="workspace-unanswered-retention-error"
            role="alert"
            className="text-sm text-destructive sm:col-start-2"
            data-testid="workspace-unanswered-retention-error"
          >
            {t("workspaces:unansweredRetentionInvalid")}
          </p>
        )}
      </div>
    </SettingsGroup>
  );
}
