"use client";

import { useTranslation } from "react-i18next";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { SettingsCard } from "@/components/settings/settings-card";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import { SETTINGS_MENU_MODES, type SettingsMenuMode } from "@/lib/settings/settings-menu-mode";

/**
 * Copy per option. Keys, not text: `t()` has to resolve at render, and a
 * module-level `t()` would freeze at the boot locale.
 */
const MODE_COPY: Record<SettingsMenuMode, { labelKey: string; descriptionKey: string }> = {
  flat: {
    labelKey: "settings:settingsMenuFlat",
    descriptionKey: "settings:settingsMenuFlatDescription",
  },
  accordion: {
    labelKey: "settings:settingsMenuAccordion",
    descriptionKey: "settings:settingsMenuAccordionDescription",
  },
  persistent: {
    labelKey: "settings:settingsMenuPersistent",
    descriptionKey: "settings:settingsMenuPersistentDescription",
  },
};

export function SettingsMenuModeCard({
  value,
  isDirty,
  onChange,
}: {
  value: SettingsMenuMode;
  isDirty: boolean;
  onChange: (mode: SettingsMenuMode) => void;
}) {
  const { t } = useTranslation();
  return (
    <SettingsCard
      isDirty={isDirty}
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.settingsMenuMode}
      data-testid="settings-menu-mode-card"
    >
      <CardHeader>
        <CardTitle className="text-base">{t("settings:settingsMenuShape")}</CardTitle>
      </CardHeader>
      <CardContent>
        <fieldset className="space-y-2" data-settings-dirty={isDirty}>
          <legend className="sr-only">{t("settings:settingsMenuShape")}</legend>
          <RadioGroup
            value={value}
            onValueChange={(next) => onChange(next as SettingsMenuMode)}
            className="grid gap-2"
          >
            {SETTINGS_MENU_MODES.map((mode) => (
              <Label
                key={mode}
                className="flex cursor-pointer items-start gap-3 rounded-md border p-3"
                data-testid={`settings-menu-mode-${mode}`}
              >
                <RadioGroupItem value={mode} className="mt-0.5" />
                <span>
                  <span className="block text-sm font-medium">{t(MODE_COPY[mode].labelKey)}</span>
                  <span className="block text-xs font-normal leading-5 text-muted-foreground">
                    {t(MODE_COPY[mode].descriptionKey)}
                  </span>
                </span>
              </Label>
            ))}
          </RadioGroup>
          <p className="text-xs text-muted-foreground">
            {t("settings:settingsMenuShapePerDevice")}
          </p>
        </fieldset>
      </CardContent>
    </SettingsCard>
  );
}
