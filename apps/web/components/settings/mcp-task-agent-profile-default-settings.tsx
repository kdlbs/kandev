"use client";

import { useEffect, useRef, useState } from "react";
import { CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import type { MCPTaskAgentProfileDefault } from "@/lib/types/http";
import { SettingsCard } from "./settings-card";
import { useSettingsSaveContributor } from "./settings-save-provider";

const OPTIONS: Array<{
  value: MCPTaskAgentProfileDefault;
  label: string;
  description: string;
}> = [
  {
    value: "current_task",
    label: "Current task profile",
    description:
      "The new task uses the same profile as the task that created it. Choose this when follow-up work needs the same model and agent setup. This may reuse a more expensive profile.",
  },
  {
    value: "workspace_default",
    label: "Workspace default profile",
    description:
      "The new task uses its workflow profile when one is set; otherwise it uses the default profile of the workspace receiving the task. Choose this to keep agent-created tasks on your standard workspace model and avoid accidentally reusing an expensive profile.",
  },
];

export function MCPTaskAgentProfileDefaultSettings() {
  const preference = useAppStore((state) => state.userSettings.mcpTaskAgentProfileDefault);
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
    id: "general-mcp-task-agent-profile-default",
    order: 10,
    revision: draft,
    isDirty,
    save: async (revision) => {
      const submitted = revision as MCPTaskAgentProfileDefault;
      await updateUserSettings({ mcp_task_agent_profile_default: submitted });
      setSaved(submitted);
      setUserSettings({
        ...storeApi.getState().userSettings,
        mcpTaskAgentProfileDefault: submitted,
      });
    },
    discard: () => setDraft(saved),
  });

  return (
    <SettingsCard isDirty={isDirty} data-testid="mcp-task-profile-default-card">
      <CardHeader>
        <CardTitle className="text-base">
          <h3>Profile for Tasks Created by Agents</h3>
        </CardTitle>
        <CardDescription className="space-y-1">
          <p>
            This setting decides which profile Kandev uses when an agent creates another task
            without choosing a profile.
          </p>
          <p>
            It does not affect tasks you create yourself. A profile chosen in the Create Task tool
            always wins.
          </p>
        </CardDescription>
      </CardHeader>
      <CardContent>
        <RadioGroup
          aria-label="Profile for tasks created by agents"
          value={draft}
          onValueChange={(value) => setDraft(value as MCPTaskAgentProfileDefault)}
          data-settings-dirty={isDirty}
          className="gap-3"
        >
          {OPTIONS.map((option) => {
            const labelId = `mcp-task-profile-${option.value}-label`;
            const descriptionId = `mcp-task-profile-${option.value}-description`;
            return (
              <Label
                key={option.value}
                htmlFor={`mcp-task-profile-${option.value}`}
                className="flex min-h-11 w-full min-w-0 cursor-pointer items-start gap-3 rounded-md border p-3 hover:bg-muted/30"
              >
                <RadioGroupItem
                  id={`mcp-task-profile-${option.value}`}
                  value={option.value}
                  aria-labelledby={labelId}
                  aria-describedby={descriptionId}
                  className="mt-0.5"
                />
                <span className="min-w-0 space-y-1">
                  <span id={labelId} className="block text-sm font-medium">
                    {option.label}
                  </span>
                  <span
                    id={descriptionId}
                    className="block whitespace-normal break-words text-xs text-muted-foreground"
                  >
                    {option.description}
                  </span>
                </span>
              </Label>
            );
          })}
        </RadioGroup>
      </CardContent>
    </SettingsCard>
  );
}
