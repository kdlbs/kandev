"use client";

import { useEffect, useRef, useState } from "react";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import { SettingsCard } from "./settings-card";
import { useSettingsSaveContributor } from "./settings-save-provider";
import { useTranslation } from "react-i18next";

/**
 * Edits the per-user preference that controls transcript unread dividers and
 * read-cursor updates. It participates in the shared Settings save/discard
 * lifecycle and commits the persisted preference to the app store on save.
 */
export function UnreadDividerSettings() {
  const { t } = useTranslation();
  const unreadDivider = useAppStore((state) => state.userSettings.unreadDivider);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const [saved, setSaved] = useState(unreadDivider);
  const [draft, setDraft] = useState(unreadDivider);
  const draftRef = useRef(draft);
  draftRef.current = draft;
  const isDirty = draft !== saved;

  useEffect(() => {
    setSaved((previous) => {
      if (draftRef.current === previous) setDraft(unreadDivider);
      return unreadDivider;
    });
  }, [unreadDivider]);

  useSettingsSaveContributor({
    id: "general-unread-divider",
    revision: Number(draft),
    isDirty,
    save: async (revision) => {
      const submitted = Boolean(revision);
      await updateUserSettings({ unread_divider: submitted });
      setSaved(submitted);
      setUserSettings({ ...storeApi.getState().userSettings, unreadDivider: submitted });
    },
    discard: () => setDraft(saved),
  });

  return (
    <SettingsCard isDirty={isDirty} data-testid="unread-divider-settings-card">
      <CardHeader>
        <CardTitle className="text-base">{t("settings:unreadMessages")}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex min-h-11 items-center justify-between gap-4">
          <div className="min-w-0 space-y-0.5">
            <Label htmlFor="show-unread-divider">{t("settings:showNewDividerInTranscripts")}</Label>
            <p className="text-xs text-muted-foreground">
              {t("settings:markMessagesThatArrivedWhileA")}
            </p>
          </div>
          <Switch
            id="show-unread-divider"
            checked={draft}
            data-settings-dirty={isDirty}
            onCheckedChange={setDraft}
            className="shrink-0 cursor-pointer"
          />
        </div>
      </CardContent>
    </SettingsCard>
  );
}
