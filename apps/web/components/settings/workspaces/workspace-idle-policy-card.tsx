"use client";

import { useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { SettingsGroup } from "@/components/settings/settings-group";

type Props = {
  canManage: boolean;
  enabled: boolean;
  timeoutMinutes: string;
  timeoutValid: boolean;
  enabledIsDirty: boolean;
  timeoutIsDirty: boolean;
  onEnabledChange: (enabled: boolean) => void;
  onTimeoutChange: (value: string) => void;
};

export function WorkspaceIdlePolicyCard({
  canManage,
  enabled,
  timeoutMinutes,
  timeoutValid,
  enabledIsDirty,
  timeoutIsDirty,
  onEnabledChange,
  onTimeoutChange,
}: Props) {
  const { t } = useTranslation();

  return (
    <SettingsGroup
      title={t("workspaces:idlePolicyTitle")}
      description={t("workspaces:idlePolicyDescription")}
      isDirty={enabledIsDirty || timeoutIsDirty}
      contentClassName="space-y-4 divide-y-0"
      data-testid="workspace-idle-policy-card"
    >
      <div className="flex min-h-11 items-center justify-between gap-4">
        <Label htmlFor="workspace-idle-suspension" className="cursor-pointer">
          {t("workspaces:idleSuspensionEnabled")}
        </Label>
        <Switch
          id="workspace-idle-suspension"
          checked={enabled}
          onCheckedChange={onEnabledChange}
          disabled={!canManage}
          data-settings-dirty={enabledIsDirty}
          data-testid="workspace-idle-suspension-switch"
          className="[@media(pointer:coarse)]:after:-inset-y-3"
        />
      </div>
      <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_12rem] sm:items-center sm:gap-4">
        <div className="space-y-1">
          <Label htmlFor="workspace-idle-timeout">{t("workspaces:idleTimeoutMinutes")}</Label>
          <p className="text-xs text-muted-foreground">
            {t("workspaces:idlePolicyResumeDescription")}
          </p>
        </div>
        <Input
          id="workspace-idle-timeout"
          type="number"
          min={1}
          step={1}
          value={timeoutMinutes}
          disabled={!canManage || !enabled}
          aria-invalid={!timeoutValid}
          aria-describedby={!timeoutValid ? "workspace-idle-timeout-error" : undefined}
          data-settings-dirty={timeoutIsDirty}
          data-testid="workspace-idle-timeout-input"
          controlSize="compact"
          className="text-base sm:text-sm md:text-xs/relaxed [@media(pointer:coarse)]:min-h-11"
          onChange={(event) => onTimeoutChange(event.target.value)}
        />
        {!timeoutValid && (
          <p
            id="workspace-idle-timeout-error"
            role="alert"
            className="text-sm text-destructive sm:col-start-2"
            data-testid="workspace-idle-timeout-error"
          >
            {t("workspaces:idleTimeoutMustBePositive")}
          </p>
        )}
      </div>
    </SettingsGroup>
  );
}
