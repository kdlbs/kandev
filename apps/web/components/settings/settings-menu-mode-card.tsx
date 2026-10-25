"use client";

import { useTranslation } from "react-i18next";
import { Label } from "@kandev/ui/label";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import { SETTINGS_MENU_MODES, type SettingsMenuMode } from "@/lib/settings/settings-menu-mode";
import { useSettingsTargetRegistration } from "./settings-target-provider";

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
  const registerTarget = useSettingsTargetRegistration(GENERAL_SETTINGS_TARGETS.settingsMenuMode);
  return (
    <fieldset
      ref={registerTarget}
      className="space-y-2 py-3"
      data-testid="settings-menu-mode-card"
      data-settings-dirty={isDirty}
    >
      <legend className="text-sm font-medium">{t("settings:settingsMenuShape")}</legend>
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
      <p className="text-xs text-muted-foreground">{t("settings:settingsMenuShapePerDevice")}</p>
    </fieldset>
  );
}
