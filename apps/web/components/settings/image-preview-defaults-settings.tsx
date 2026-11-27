"use client";

import { useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { useSettingsSaveContributor } from "./settings-save-provider";
import { useTranslation } from "react-i18next";
import { Switch } from "@kandev/ui/switch";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import { SettingsRow } from "./settings-group";
import { SETTINGS_SWITCH_CLASS_NAME } from "./settings-control";

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
          className={SETTINGS_SWITCH_CLASS_NAME}
        />
      }
    />
  );
}
