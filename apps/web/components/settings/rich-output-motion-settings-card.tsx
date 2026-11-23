"use client";

import { useTranslation } from "react-i18next";
import { Switch } from "@kandev/ui/switch";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import { SettingsRow } from "./settings-group";
import { SETTINGS_SWITCH_CLASS_NAME } from "./settings-control";

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
      control={
        <Switch
          id="animate-rich-output-charts"
          checked={enabled}
          onCheckedChange={onChange}
          data-settings-dirty={isDirty}
          className={SETTINGS_SWITCH_CLASS_NAME}
        />
      }
    />
  );
}
