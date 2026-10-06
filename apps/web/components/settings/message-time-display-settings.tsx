"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import { isUserSettingsResponseCurrent } from "@/lib/settings/user-settings-revision";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import type { MessageTimeDisplay } from "@/lib/types/http-user-settings";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import { SettingsRow } from "./settings-group";
import { SettingsSaveCancelledError, useSettingsSaveContributor } from "./settings-save-provider";

const MESSAGE_TIME_DISPLAY_OPTIONS: readonly {
  value: MessageTimeDisplay;
  labelKey: string;
}[] = [
  { value: "relative", labelKey: "settings:messageTimeRelative" },
  { value: "absolute_short", labelKey: "settings:messageTimeAbsoluteShort" },
  { value: "absolute_long", labelKey: "settings:messageTimeAbsoluteLong" },
];

export function MessageTimeDisplaySettings() {
  const { t } = useTranslation();
  const preference = useAppStore((state) => state.userSettings.messageTimeDisplay);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const [saved, setSaved] = useState(preference);
  const [draft, setDraft] = useState(preference);
  const draftRef = useRef(draft);
  draftRef.current = draft;
  const isDirty = draft !== saved;

  useEffect(() => {
    setSaved((previous) => {
      if (draftRef.current === previous) setDraft(preference);
      return preference;
    });
  }, [preference]);

  useSettingsSaveContributor({
    id: "general-message-time-display",
    order: 22,
    revision: draft,
    isDirty,
    save: async (revision) => {
      const submitted = revision as MessageTimeDisplay;
      const settingsAtSubmit = storeApi.getState().userSettings;
      const response = await updateUserSettings({ message_time_display: submitted });
      const state = storeApi.getState();
      if (
        !isUserSettingsResponseCurrent(
          response.settings.revision,
          state.userSettings.revision,
          state.userSettings === settingsAtSubmit,
        )
      ) {
        throw new SettingsSaveCancelledError();
      }
      setSaved(submitted);
      setUserSettings(mapUserSettingsResponse(response, state.userSettings));
    },
    discard: () => setDraft(saved),
  });

  return (
    <SettingsRow
      label={t("settings:messageTimeDisplay")}
      description={t("settings:messageTimeDisplayDescription")}
      descriptionId="message-time-display-description"
      controlId="message-time-display"
      isDirty={isDirty}
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.messageTimeDisplay}
      data-testid="message-time-display-settings-card"
      control={
        <Select value={draft} onValueChange={(value) => setDraft(value as MessageTimeDisplay)}>
          <SelectTrigger
            id="message-time-display"
            data-settings-dirty={isDirty}
            aria-describedby="message-time-display-description"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {MESSAGE_TIME_DISPLAY_OPTIONS.map(({ value, labelKey }) => (
              <SelectItem key={value} value={value}>
                {t(labelKey)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      }
    />
  );
}
