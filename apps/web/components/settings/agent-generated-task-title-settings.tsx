"use client";

import { useEffect, useRef, useState } from "react";
import { CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import { SettingsCard } from "./settings-card";
import { useSettingsSaveContributor } from "./settings-save-provider";

export function AgentGeneratedTaskTitleSettings() {
  const preference = useAppStore((state) => state.userSettings.agentGeneratedTaskTitles);
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
    id: "general-agent-generated-task-titles",
    order: 20,
    revision: Number(draft),
    isDirty,
    save: async (revision) => {
      const submitted = Boolean(revision);
      await updateUserSettings({ agent_generated_task_titles: submitted });
      setSaved(submitted);
      setUserSettings({ ...storeApi.getState().userSettings, agentGeneratedTaskTitles: submitted });
    },
    discard: () => setDraft(saved),
  });

  return (
    <SettingsCard isDirty={isDirty} data-testid="agent-generated-task-title-card">
      <CardHeader>
        <CardTitle className="text-base">Agent-generated task titles</CardTitle>
        <CardDescription>
          When enabled, new task and subtask forms hide the manual title field and require a prompt.
          Kandev shows a provisional title using the prompt&apos;s first six words immediately, then
          asks the agent to replace it with a concise title targeting about three words. If the
          agent cannot rename the task, the provisional title remains available for you to edit.
          Existing and edited tasks keep their normal title field.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="flex min-h-11 items-center justify-between gap-4">
          <div className="min-w-0 space-y-0.5">
            <Label htmlFor="agent-generated-task-titles">Use the agent for new task titles</Label>
            <p className="text-xs text-muted-foreground">
              Disabled by default; save this setting before opening a creation dialog.
            </p>
          </div>
          <Switch
            id="agent-generated-task-titles"
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
