"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import { SettingsRow } from "./settings-group";
import { settingsControlClassName } from "./settings-control";
import { useSettingsSaveContributor } from "./settings-save-provider";

export const UNREAD_TASK_DOT_HIDE_DELAY_MAX_SECONDS = 60;

/** Parses the delay field; whole seconds from 0 to 60, otherwise null. */
export function parseUnreadTaskDotHideDelay(value: string): number | null {
  if (!/^\d+$/.test(value)) return null;
  const seconds = Number(value);
  return seconds <= UNREAD_TASK_DOT_HIDE_DELAY_MAX_SECONDS ? seconds : null;
}

/**
 * Edits how long an opened task keeps its unread dot before it clears. It
 * participates in the shared Settings save/discard lifecycle and commits the
 * persisted preference to the app store on save. It stays mounted while
 * hidden so an unsaved edit and its save registration survive the dot switch
 * being toggled.
 */
export function UnreadTaskDotHideDelaySettings({ visible }: { visible: boolean }) {
  const { t } = useTranslation();
  const stored = useAppStore((state) => state.userSettings.unreadTaskDotHideDelaySeconds);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const [saved, setSaved] = useState(String(stored));
  const [draft, setDraft] = useState(String(stored));
  const draftRef = useRef(draft);
  draftRef.current = draft;
  const isDirty = draft !== saved;
  const seconds = parseUnreadTaskDotHideDelay(draft);

  useEffect(() => {
    setSaved((previous) => {
      if (draftRef.current === previous) setDraft(String(stored));
      return String(stored);
    });
  }, [stored]);

  useSettingsSaveContributor({
    id: "general-unread-task-dot-hide-delay",
    revision: draft,
    isDirty,
    canSave: seconds !== null,
    invalidReason: seconds === null ? t("settings:unreadTaskDotHideDelayError") : undefined,
    save: async (revision) => {
      const submitted = parseUnreadTaskDotHideDelay(String(revision));
      if (submitted === null) return;
      await updateUserSettings({ unread_task_dot_hide_delay_seconds: submitted });
      setSaved(String(submitted));
      setUserSettings({
        ...storeApi.getState().userSettings,
        unreadTaskDotHideDelaySeconds: submitted,
      });
    },
    discard: () => setDraft(saved),
  });

  if (!visible) return null;
  return (
    <SettingsRow
      label={t("settings:unreadTaskDotHideDelay")}
      description={
        seconds === null ? (
          <span id="unread-task-dot-hide-delay-error" className="text-destructive" role="alert">
            {t("settings:unreadTaskDotHideDelayError")}
          </span>
        ) : (
          <span id="unread-task-dot-hide-delay-help">
            {t("settings:unreadTaskDotHideDelayHelp")}
          </span>
        )
      }
      controlId="unread-task-dot-hide-delay"
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.unreadTaskDotHideDelay}
      isDirty={isDirty}
      control={
        <Input
          id="unread-task-dot-hide-delay"
          type="number"
          inputMode="numeric"
          min={0}
          max={UNREAD_TASK_DOT_HIDE_DELAY_MAX_SECONDS}
          step={1}
          value={draft}
          aria-invalid={seconds === null}
          aria-describedby={
            seconds === null
              ? "unread-task-dot-hide-delay-error"
              : "unread-task-dot-hide-delay-help"
          }
          data-settings-dirty={isDirty}
          className={settingsControlClassName("md:max-w-40")}
          onChange={(event) => setDraft(event.target.value)}
        />
      }
    />
  );
}
