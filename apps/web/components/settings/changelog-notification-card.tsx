"use client";

import { useState } from "react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Separator } from "@kandev/ui/separator";
import { Switch } from "@kandev/ui/switch";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import { useSettingsSaveContributor } from "./settings-save-provider";

export function ChangelogNotificationCard() {
  const userSettings = useAppStore((state) => state.userSettings);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const [saved, setSaved] = useState(userSettings.showReleaseNotification);
  const [draft, setDraft] = useState(saved);
  const [isResetting, setIsResetting] = useState(false);

  useSettingsSaveContributor({
    id: "changelog-notifications",
    revision: String(draft),
    isDirty: draft !== saved,
    save: async () => {
      const submitted = draft;
      await updateUserSettings({ show_release_notification: submitted });
      setSaved(submitted);
      setUserSettings({ ...storeApi.getState().userSettings, showReleaseNotification: submitted });
    },
    discard: () => setDraft(saved),
  });

  const resetLastSeen = async () => {
    setIsResetting(true);
    try {
      await updateUserSettings({ release_notes_last_seen_version: "" });
      setUserSettings({
        ...storeApi.getState().userSettings,
        releaseNotesLastSeenVersion: null,
      });
    } catch {
      // The immediate command leaves the current persisted marker unchanged.
    } finally {
      setIsResetting(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Topbar Release Notification</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex items-center justify-between">
          <div className="space-y-0.5">
            <Label htmlFor="release-notification-toggle">Show notification for new releases</Label>
            <p className="text-xs text-muted-foreground">
              When enabled, a sparkle icon appears in the topbar when a new version is released
            </p>
          </div>
          <Switch
            id="release-notification-toggle"
            checked={draft}
            onCheckedChange={setDraft}
            className="cursor-pointer"
          />
        </div>
        <Separator className="my-4" />
        <div className="flex items-center justify-between">
          <div className="space-y-0.5">
            <Label>Reset seen releases</Label>
            <p className="text-xs text-muted-foreground">
              Clear your last seen version so the topbar notification appears again
            </p>
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={() => void resetLastSeen()}
            disabled={isResetting || !userSettings.releaseNotesLastSeenVersion}
            className="cursor-pointer"
          >
            Reset
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
