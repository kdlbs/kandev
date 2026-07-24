"use client";

import { useEffect, useRef, useState } from "react";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Switch } from "@kandev/ui/switch";
import { useAppStore } from "@/components/state-provider";
import { SettingsCard } from "@/components/settings/settings-card";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { useToast } from "@/components/toast-provider";
import { saveUpdateNotificationSettings } from "@/lib/api/domains/system-api";
import type { UpdateNotificationChannel, UpdateNotificationSettings } from "@/lib/types/system";

// Matches the backend default (DefaultNotifySettings). Only used while the
// initial settings fetch is pending (no SSR-hydrated value was available).
const DEFAULT_SETTINGS: UpdateNotificationSettings = { enabled: true, channel: "both" };

const CHANNEL_OPTIONS: Array<{ value: UpdateNotificationChannel; label: string; help: string }> = [
  {
    value: "desktop",
    label: "Desktop notification",
    help: "Shows a native OS notification (or a browser notification when this isn't the desktop app).",
  },
  {
    value: "in_view",
    label: "In-app banner",
    help: "Shows an in-app banner only while Kandev is open in this window.",
  },
  {
    value: "both",
    label: "Both",
    help: "Shows both a desktop notification and an in-app banner.",
  },
];

function useUpdateNotificationSettingsDraft() {
  const stored = useAppStore((s) => s.system.updateNotificationSettings);
  const setStored = useAppStore((s) => s.setSystemUpdateNotificationSettings);
  const { toast } = useToast();
  const [saved, setSaved] = useState<UpdateNotificationSettings>(stored ?? DEFAULT_SETTINGS);
  const [draft, setDraft] = useState<UpdateNotificationSettings>(saved);
  // Boot-payload hydration (see settings-routes.tsx: loadSettingsInitialState)
  // resolves asynchronously and may land after this component's first
  // render. Sync draft/saved from the store exactly once, the first time a
  // real value shows up, so an in-flight boot fetch doesn't get shadowed by
  // DEFAULT_SETTINGS. Never resync afterward so in-progress edits survive
  // unrelated store updates (including this hook's own save round-trip).
  const hydratedFromStoreRef = useRef(stored !== null);

  useEffect(() => {
    if (hydratedFromStoreRef.current || stored === null) return;
    hydratedFromStoreRef.current = true;
    setSaved(stored);
    setDraft(stored);
  }, [stored]);

  const revision = JSON.stringify(draft);
  const isDirty = revision !== JSON.stringify(saved);

  useSettingsSaveContributor({
    id: "system-update-notifications",
    revision,
    isDirty,
    save: async () => {
      const submitted = draft;
      const persisted = await saveUpdateNotificationSettings(submitted);
      setSaved(persisted);
      setDraft(persisted);
      setStored(persisted);
      toast({ title: "Update notification settings saved", variant: "success" });
    },
    discard: () => setDraft(saved),
  });

  return { draft, saved, setDraft, isDirty };
}

export function UpdateNotificationsCard() {
  const { draft, saved, setDraft, isDirty } = useUpdateNotificationSettingsDraft();
  const activeChannel = CHANNEL_OPTIONS.find((option) => option.value === draft.channel);

  return (
    <SettingsCard isDirty={isDirty} data-testid="update-notifications-card">
      <CardHeader>
        <CardTitle>Update notifications</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-center justify-between gap-4">
          <div>
            <div className="text-base font-medium">Notify me about new releases</div>
            <p className="text-sm text-muted-foreground">
              Kandev checks GitHub for new releases every few hours in the background. When a newer
              version is found, show a one-time notification for that release below.
            </p>
          </div>
          <Switch
            checked={draft.enabled}
            data-settings-dirty={draft.enabled !== saved.enabled}
            onCheckedChange={(enabled) => setDraft({ ...draft, enabled })}
            aria-label="Enable update notifications"
            className="cursor-pointer"
          />
        </div>
        {draft.enabled && (
          <div className="flex items-center gap-2">
            <Select
              value={draft.channel}
              onValueChange={(channel) =>
                setDraft({ ...draft, channel: channel as UpdateNotificationChannel })
              }
            >
              <SelectTrigger
                className="w-56 cursor-pointer"
                data-settings-dirty={draft.channel !== saved.channel}
                aria-label="Update notification channel"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {CHANNEL_OPTIONS.map((option) => (
                  <SelectItem key={option.value} value={option.value} className="cursor-pointer">
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-sm text-muted-foreground">{activeChannel?.help}</p>
          </div>
        )}
      </CardContent>
    </SettingsCard>
  );
}
