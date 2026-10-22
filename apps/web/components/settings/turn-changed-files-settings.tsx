"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Switch } from "@kandev/ui/switch";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import { isUserSettingsResponseCurrent } from "@/lib/settings/user-settings-revision";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import { SettingsRow } from "./settings-group";
import { useSettingsSaveContributor } from "./settings-save-provider";

export function TurnChangedFilesSettings() {
  const { t } = useTranslation();
  const showTurnChangedFiles = useAppStore((state) => state.userSettings.showTurnChangedFiles);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const [saved, setSaved] = useState(showTurnChangedFiles);
  const [draft, setDraft] = useState(showTurnChangedFiles);
  const draftRef = useRef(draft);
  const savingRef = useRef(false);
  const editedDuringSaveRef = useRef(false);
  draftRef.current = draft;
  const isDirty = draft !== saved;

  useEffect(() => {
    if (savingRef.current) return;
    setSaved((previous) => {
      if (draftRef.current === previous) setDraft(showTurnChangedFiles);
      return showTurnChangedFiles;
    });
  }, [showTurnChangedFiles]);

  useSettingsSaveContributor({
    id: "general-turn-changed-files",
    revision: Number(draft),
    isDirty,
    save: async () => {
      const submitted = draft;
      const before = storeApi.getState().userSettings;
      editedDuringSaveRef.current = false;
      savingRef.current = true;
      try {
        const response = await updateUserSettings({ show_turn_changed_files: submitted });
        const current = storeApi.getState().userSettings;
        const next = isUserSettingsResponseCurrent(
          response.settings.revision,
          current.revision,
          current === before,
        )
          ? mapUserSettingsResponse(response, current)
          : current;
        setSaved(next.showTurnChangedFiles);
        setUserSettings(next);
        if (!editedDuringSaveRef.current) setDraft(next.showTurnChangedFiles);
      } finally {
        savingRef.current = false;
      }
    },
    discard: () => setDraft(saved),
  });

  return (
    <SettingsRow
      data-testid="turn-changed-files-settings-row"
      label={t("settings:showTurnChangedFiles")}
      description={t("settings:showTurnChangedFilesDescription")}
      controlId="show-turn-changed-files"
      touchTarget="switch"
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.turnChangedFiles}
      isDirty={isDirty}
      control={
        <Switch
          id="show-turn-changed-files"
          checked={draft}
          data-settings-dirty={isDirty}
          onCheckedChange={(enabled) => {
            if (savingRef.current && enabled !== draftRef.current) {
              editedDuringSaveRef.current = true;
            }
            setDraft(enabled);
          }}
          className="shrink-0 cursor-pointer"
        />
      }
    />
  );
}
