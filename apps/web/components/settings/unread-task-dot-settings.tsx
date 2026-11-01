"use client";
import { SettingsInfo } from "./settings-info";

import { useEffect, useRef, useState } from "react";
import { Switch } from "@kandev/ui/switch";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import { SettingsRow } from "./settings-group";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import { useSettingsSaveContributor } from "./settings-save-provider";
import { UnreadTaskDotHideDelaySettings } from "./unread-task-dot-hide-delay-settings";
import { useTranslation } from "react-i18next";

/**
 * Edits the per-user preference that shows a dot on task rows whose sessions
 * have unread messages, and, while the switch is on, the dot's hide delay. It
 * participates in the shared Settings save/discard lifecycle and commits the
 * persisted preference to the app store on save.
 */
export function UnreadTaskDotSettings() {
  const { t } = useTranslation();
  const unreadTaskDot = useAppStore((state) => state.userSettings.unreadTaskDot);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const [saved, setSaved] = useState(unreadTaskDot);
  const [draft, setDraft] = useState(unreadTaskDot);
  const draftRef = useRef(draft);
  draftRef.current = draft;
  const isDirty = draft !== saved;

  useEffect(() => {
    setSaved((previous) => {
      if (draftRef.current === previous) setDraft(unreadTaskDot);
      return unreadTaskDot;
    });
  }, [unreadTaskDot]);

  useSettingsSaveContributor({
    id: "general-unread-task-dot",
    revision: Number(draft),
    isDirty,
    save: async (revision) => {
      const submitted = Boolean(revision);
      await updateUserSettings({ unread_task_dot: submitted });
      setSaved(submitted);
      setUserSettings({ ...storeApi.getState().userSettings, unreadTaskDot: submitted });
    },
    discard: () => setDraft(saved),
  });
  return (
    <>
      <SettingsRow
        label={t("settings:showUnreadDotOnTasks")}
        description={t("settings:unreadTaskDotShort")}
        info={
          <SettingsInfo label={t("settings:showUnreadDotOnTasks")}>
            {t("settings:unreadTaskDotInfo")}
          </SettingsInfo>
        }
        controlId="show-unread-task-dot"
        touchTarget="switch"
        discoveryTargetId={GENERAL_SETTINGS_TARGETS.unreadTaskDot}
        isDirty={isDirty}
        control={
          <Switch
            id="show-unread-task-dot"
            checked={draft}
            data-settings-dirty={isDirty}
            onCheckedChange={setDraft}
            className="shrink-0 cursor-pointer"
          />
        }
      />
      <UnreadTaskDotHideDelaySettings visible={draft} />
    </>
  );
}
