"use client";

import { useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { SettingsGroup } from "@/components/settings/settings-group";

type Props = {
  canManage: boolean;
  enabled: boolean;
  graceMinutes: string;
  graceValid: boolean;
  enabledIsDirty: boolean;
  graceIsDirty: boolean;
  onEnabledChange: (enabled: boolean) => void;
  onGraceChange: (value: string) => void;
};

export function WorkspaceWipReleaseCard({
  canManage,
  enabled,
  graceMinutes,
  graceValid,
  enabledIsDirty,
  graceIsDirty,
  onEnabledChange,
  onGraceChange,
}: Props) {
  const { t } = useTranslation();

  return (
    <SettingsGroup
      title={t("workspaces:wipReleaseTitle")}
      description={t("workspaces:wipReleaseDescription")}
      isDirty={enabledIsDirty || graceIsDirty}
      contentClassName="space-y-4 divide-y-0"
      data-testid="workspace-wip-release-card"
    >
      <div className="flex min-h-11 items-center justify-between gap-4">
        <Label htmlFor="workspace-wip-release" className="cursor-pointer">
          {t("workspaces:wipReleaseEnabled")}
        </Label>
        <Switch
          id="workspace-wip-release"
          checked={enabled}
          onCheckedChange={onEnabledChange}
          disabled={!canManage}
          data-settings-dirty={enabledIsDirty}
          data-testid="workspace-wip-release-switch"
          className="[@media(pointer:coarse)]:after:-inset-y-3"
        />
      </div>
      <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_12rem] sm:items-center sm:gap-4">
        <div className="space-y-1">
          <Label htmlFor="workspace-wip-release-grace">
            {t("workspaces:wipReleaseGraceMinutes")}
          </Label>
          <p className="text-xs text-muted-foreground">{t("workspaces:wipReleaseGraceHint")}</p>
        </div>
        <Input
          id="workspace-wip-release-grace"
          type="number"
          min={1}
          step={1}
          value={graceMinutes}
          disabled={!canManage || !enabled}
          aria-invalid={!graceValid}
          aria-describedby={!graceValid ? "workspace-wip-release-grace-error" : undefined}
          data-settings-dirty={graceIsDirty}
          data-testid="workspace-wip-release-grace-input"
          controlSize="compact"
          className="text-base sm:text-sm md:text-xs/relaxed [@media(pointer:coarse)]:min-h-11"
          onChange={(event) => onGraceChange(event.target.value)}
        />
        {!graceValid && (
          <p
            id="workspace-wip-release-grace-error"
            role="alert"
            className="text-sm text-destructive sm:col-start-2"
            data-testid="workspace-wip-release-grace-error"
          >
            {t("workspaces:wipReleaseGraceMustBePositive")}
          </p>
        )}
      </div>
    </SettingsGroup>
  );
}
