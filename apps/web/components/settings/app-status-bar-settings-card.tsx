"use client";

import { useTranslation } from "react-i18next";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import { SettingsCard } from "./settings-card";

export function AppStatusBarSettingsCard({
  enabled,
  isDirty,
  onChange,
}: {
  enabled: boolean;
  isDirty: boolean;
  onChange: (enabled: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <SettingsCard
      isDirty={isDirty}
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.appStatusBar}
      data-testid="app-status-bar-settings-card"
    >
      <CardHeader>
        <CardTitle className="text-base">{t("settings:statusBar")}</CardTitle>
      </CardHeader>
      <CardContent>
        <div
          className="flex min-h-11 items-center justify-between gap-4"
          data-testid="app-status-bar-toggle-row"
        >
          <div className="space-y-1">
            <Label htmlFor="show-app-status-bar">{t("settings:showStatusBar")}</Label>
            <p className="max-w-3xl text-xs text-muted-foreground">
              {t("settings:showStatusBarDescription")}
            </p>
          </div>
          <Switch
            id="show-app-status-bar"
            checked={enabled}
            onCheckedChange={onChange}
            data-settings-dirty={isDirty}
            className="data-checked:bg-transparent data-unchecked:bg-transparent dark:data-unchecked:bg-transparent data-[size=default]:h-11 data-[size=default]:w-11 cursor-pointer p-2 before:absolute before:left-2 before:top-1/2 before:h-[16.6px] before:w-7 before:-translate-y-1/2 before:rounded-full before:bg-input before:content-[''] data-checked:before:bg-primary dark:data-unchecked:before:bg-input/80 [&_[data-slot=switch-thumb]]:z-10"
          />
        </div>
      </CardContent>
    </SettingsCard>
  );
}
