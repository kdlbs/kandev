"use client";

import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { updateWorkspaceAction } from "@/app/actions/workspaces";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useWorkspaces } from "@/hooks/domains/workspace/use-workspaces";
import { patchWorkspaceCache } from "@/lib/query/workspace-cache";
import { agentProfileId as toAgentProfileId } from "@/lib/types/ids";
import { useSettingsSaveContributor } from "./settings-save-provider";
import { SettingsCard } from "./settings-card";

export function ConfigChatAgentSection() {
  const { activeWorkspace: workspace } = useWorkspaces();
  const { agentProfiles: profiles } = useSettingsData(true);
  const currentProfileId = workspace?.default_config_agent_profile_id ?? "";
  const workspaceId = workspace?.id ?? null;
  const [syncedWorkspaceId, setSyncedWorkspaceId] = useState(workspaceId);
  const [savedProfileId, setSavedProfileId] = useState(currentProfileId);
  const [draftProfileId, setDraftProfileId] = useState(currentProfileId);
  const queryClient = useQueryClient();
  const isDirty = draftProfileId !== savedProfileId;

  useEffect(() => {
    if (workspaceId !== syncedWorkspaceId) {
      setSyncedWorkspaceId(workspaceId);
      setSavedProfileId(currentProfileId);
      setDraftProfileId(currentProfileId);
      return;
    }
    if (isDirty) return;
    setSavedProfileId(currentProfileId);
    setDraftProfileId(currentProfileId);
  }, [currentProfileId, isDirty, syncedWorkspaceId, workspaceId]);

  useSettingsSaveContributor({
    id: "utility-config-chat-agent",
    order: 20,
    revision: draftProfileId,
    isDirty: Boolean(workspace) && isDirty,
    save: async () => {
      if (!workspace) return;
      const submitted = draftProfileId;
      await updateWorkspaceAction(workspace.id, {
        default_config_agent_profile_id: submitted,
      });
      patchWorkspaceCache(queryClient, workspace.id, {
        default_config_agent_profile_id: submitted ? toAgentProfileId(submitted) : null,
      });
      setSavedProfileId(submitted);
    },
    discard: () => setDraftProfileId(savedProfileId),
  });

  if (!workspace) return null;

  return (
    <SettingsCard isDirty={isDirty} data-testid="config-chat-agent-card">
      <CardHeader>
        <CardTitle className="text-base">
          <h3>Configuration Chat Agent</h3>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-sm text-muted-foreground">
          Choose which agent profile to use for the Configuration Chat. This agent can manage your
          workflows, agent profiles, and MCP configuration.
        </p>
        <Select
          value={draftProfileId || "none"}
          onValueChange={(value) => setDraftProfileId(value === "none" ? "" : value)}
        >
          <SelectTrigger className="w-full max-w-sm cursor-pointer" data-settings-dirty={isDirty}>
            <SelectValue placeholder="Choose an agent profile..." />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="none">No default</SelectItem>
            {profiles.map((p) => (
              <SelectItem key={p.id} value={p.id} className="cursor-pointer">
                {p.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </CardContent>
    </SettingsCard>
  );
}
