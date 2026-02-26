"use client";

import { useState } from "react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Separator } from "@kandev/ui/separator";
import { Switch } from "@kandev/ui/switch";
import { useAppStore } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";

export function ChangelogNotificationCard() {
  const userSettings = useAppStore((state) => state.userSettings);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const [isSaving, setIsSaving] = useState(false);

  const handleToggle = async (checked: boolean) => {
    if (isSaving) return;
    setIsSaving(true);
    const previousValue = userSettings.showReleaseNotification;
    try {
      setUserSettings({ ...userSettings, showReleaseNotification: checked });
      await updateUserSettings({ show_release_notification: checked });
    } catch {
      setUserSettings({ ...userSettings, showReleaseNotification: previousValue });
    } finally {
      setIsSaving(false);
    }
  };

  const handleResetLastSeen = async () => {
    if (isSaving) return;
    setIsSaving(true);
    try {
      setUserSettings({ ...userSettings, releaseNotesLastSeenVersion: null });
      await updateUserSettings({ release_notes_last_seen_version: "" });
    } catch {
      // Ignore — worst case the button stays hidden until next release
    } finally {
      setIsSaving(false);
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
            checked={userSettings.showReleaseNotification}
            onCheckedChange={handleToggle}
            disabled={isSaving}
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
            onClick={handleResetLastSeen}
            disabled={isSaving || !userSettings.releaseNotesLastSeenVersion}
            className="cursor-pointer"
          >
            Reset
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
