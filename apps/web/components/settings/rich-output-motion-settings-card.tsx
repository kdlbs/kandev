"use client";

import { useTranslation } from "react-i18next";
import { Switch } from "@kandev/ui/switch";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import { SettingsRow } from "./settings-group";

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
    <SettingsRow
      isDirty={isDirty}
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.richOutputMotion}
      data-testid="rich-output-motion-settings-card"
      label={t("settings:animateRichOutputCharts")}
      description={t("settings:animateRichOutputChartsDescription")}
      controlId="animate-rich-output-charts"
      touchTarget="switch"
      controlWrapperTestId="rich-output-motion-toggle-row"
      controlWrapperClassName="min-h-11"
      control={
        <Switch
          id="animate-rich-output-charts"
          checked={enabled}
          onCheckedChange={onChange}
          data-settings-dirty={isDirty}
          className="data-[state=checked]:bg-transparent data-[state=unchecked]:bg-transparent dark:data-[state=unchecked]:bg-transparent data-[size=default]:h-11 data-[size=default]:w-11 cursor-pointer shrink-0 p-2 before:absolute before:left-2 before:top-1/2 before:h-[16.6px] before:w-7 before:-translate-y-1/2 before:rounded-full before:bg-input before:content-[''] data-[state=checked]:before:bg-primary dark:data-[state=unchecked]:before:bg-input/80 [&_[data-slot=switch-thumb]]:z-10"
        />
      }
    />
  );
}
