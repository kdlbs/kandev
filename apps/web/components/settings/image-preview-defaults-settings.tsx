"use client";

import { useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { useSettingsSaveContributor } from "./settings-save-provider";
import { useTranslation } from "react-i18next";
import { Switch } from "@kandev/ui/switch";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import { SettingsRow } from "./settings-group";

export function ImagePreviewDefaultsSettings() {
  const saved = useAppStore((state) => state.imagePreviewsExpandedByDefault);
  const commit = useAppStore((state) => state.commitImagePreviewsExpandedByDefault);
  const [enabled, setEnabled] = useState(saved);
  const isDirty = enabled !== saved;
  useSettingsSaveContributor({
    id: "general-image-preview-defaults",
    order: 11,
    revision: String(enabled),
    isDirty,
    save: async () => {
      commit(enabled);
    },
    discard: () => setEnabled(saved),
  });
  const { t } = useTranslation();
  return (
    <SettingsRow
      isDirty={isDirty}
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.imagePreviewDefaults}
      data-testid="image-preview-defaults-settings"
      label={t("settings:showImagePreviewsByDefault")}
      description={t("settings:showImagePreviewsByDefaultDescription")}
      controlId="show-image-previews-by-default"
      touchTarget="switch"
      controlWrapperTestId="image-preview-defaults-toggle-row"
      control={
        <Switch
          id="show-image-previews-by-default"
          checked={enabled}
          onCheckedChange={setEnabled}
          data-settings-dirty={isDirty}
          className="data-[state=checked]:bg-transparent data-[state=unchecked]:bg-transparent dark:data-[state=unchecked]:bg-transparent data-[size=default]:h-7 data-[size=default]:w-9 [@media(pointer:coarse)]:data-[size=default]:h-11 [@media(pointer:coarse)]:data-[size=default]:w-11 cursor-pointer shrink-0 p-1 [@media(pointer:coarse)]:p-2 before:absolute before:left-1 [@media(pointer:coarse)]:before:left-2 before:top-1/2 before:h-[16.6px] before:w-7 before:-translate-y-1/2 before:rounded-full before:bg-input before:content-[''] data-[state=checked]:before:bg-primary dark:data-[state=unchecked]:before:bg-input/80 [&_[data-slot=switch-thumb]]:z-10"
        />
      }
    />
  );
}
