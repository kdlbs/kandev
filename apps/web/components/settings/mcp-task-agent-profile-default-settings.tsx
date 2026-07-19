"use client";

import { useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import type { MCPTaskAgentProfileDefault } from "@/lib/types/http";

const OPTIONS: Array<{
  value: MCPTaskAgentProfileDefault;
  label: string;
  description: string;
}> = [
  {
    value: "current_task",
    label: "Current task profile",
    description: "Inherit the agent profile from the task making the MCP request.",
  },
  {
    value: "workspace_default",
    label: "Workspace default profile",
    description:
      "Use the workflow profile first, then the default agent profile for the target workspace.",
  },
];

export function MCPTaskAgentProfileDefaultSettings() {
  const preference = useAppStore((state) => state.userSettings.mcpTaskAgentProfileDefault);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const [isSaving, setIsSaving] = useState(false);

  const handleChange = async (value: string) => {
    if (isSaving || value === preference) return;

    const next = value as MCPTaskAgentProfileDefault;
    const state = storeApi.getState();
    const current = state.userSettings;
    const previous = current.mcpTaskAgentProfileDefault;
    const serverRevision = state.userSettingsServerRevision;
    setIsSaving(true);
    setUserSettings({ ...current, mcpTaskAgentProfileDefault: next });

    try {
      await updateUserSettings({ mcp_task_agent_profile_default: next });
    } catch {
      const latest = storeApi.getState();
      if (latest.userSettingsServerRevision === serverRevision) {
        setUserSettings({ ...latest.userSettings, mcpTaskAgentProfileDefault: previous });
      }
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">MCP-Created Task Profile</CardTitle>
      </CardHeader>
      <CardContent>
        <RadioGroup
          aria-label="Default agent profile for MCP-created tasks"
          value={preference}
          onValueChange={handleChange}
          disabled={isSaving}
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
    </Card>
  );
}
