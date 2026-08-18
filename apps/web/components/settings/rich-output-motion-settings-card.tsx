"use client";

import { useTranslation } from "react-i18next";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import { SettingsCard } from "./settings-card";

export function RichOutputMotionSettingsCard({
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
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.richOutputMotion}
      data-testid="rich-output-motion-settings-card"
    >
      <CardHeader>
        <CardTitle className="text-base">{t("settings:richOutputChartMotion")}</CardTitle>
      </CardHeader>
      <CardContent>
        <div
          className="flex min-h-11 items-center justify-between gap-4"
          data-testid="rich-output-motion-toggle-row"
        >
          <div className="min-w-0 space-y-1">
            <Label htmlFor="animate-rich-output-charts">
              {t("settings:animateRichOutputCharts")}
            </Label>
            <p className="max-w-3xl text-xs text-muted-foreground">
              {t("settings:animateRichOutputChartsDescription")}
            </p>
          </div>
          <Switch
            id="animate-rich-output-charts"
            checked={enabled}
            onCheckedChange={onChange}
            data-settings-dirty={isDirty}
            className="data-[state=checked]:bg-transparent data-[state=unchecked]:bg-transparent dark:data-[state=unchecked]:bg-transparent data-[size=default]:h-11 data-[size=default]:w-11 cursor-pointer shrink-0 p-2 before:absolute before:left-2 before:top-1/2 before:h-[16.6px] before:w-7 before:-translate-y-1/2 before:rounded-full before:bg-input before:content-[''] data-[state=checked]:before:bg-primary dark:data-[state=unchecked]:before:bg-input/80 [&_[data-slot=switch-thumb]]:z-10"
          />
        </div>
      </CardContent>
    </SettingsCard>
  );
}
