"use client";

import { useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";

export function ArchiveConfirmationSettings() {
  const confirmTaskArchive = useAppStore((state) => state.userSettings.confirmTaskArchive);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const [isSaving, setIsSaving] = useState(false);

  const handleToggle = async (checked: boolean) => {
    if (isSaving) return;

    const current = storeApi.getState().userSettings;
    const previous = current.confirmTaskArchive;
    setIsSaving(true);
    setUserSettings({ ...current, confirmTaskArchive: checked });

    try {
      await updateUserSettings({ confirm_task_archive: checked });
    } catch {
      const latest = storeApi.getState().userSettings;
      setUserSettings({ ...latest, confirmTaskArchive: previous });
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Archive Confirmation</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex min-h-11 items-center justify-between gap-4">
          <div className="min-w-0 space-y-0.5">
            <Label htmlFor="confirm-task-archive">Confirm before archiving tasks</Label>
            <p className="text-xs text-muted-foreground">
              Show cleanup details and subtask options before an archive starts.
            </p>
          </div>
          <Switch
            id="confirm-task-archive"
            checked={confirmTaskArchive}
            onCheckedChange={handleToggle}
            disabled={isSaving}
            className="shrink-0 cursor-pointer"
          />
        </div>
      </CardContent>
    </Card>
  );
}
