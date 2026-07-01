"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Separator } from "@kandev/ui/separator";
import { useToast } from "@/components/toast-provider";
import { updateWorkspaceAction } from "@/app/actions/workspaces";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useWorkspaces } from "@/hooks/domains/workspace/use-workspaces";
import { patchWorkspaceCache } from "@/lib/query/workspace-cache";
import { agentProfileId as toAgentProfileId } from "@/lib/types/ids";

export function ConfigChatAgentSection() {
  const { activeWorkspace: workspace } = useWorkspaces();
  const { agentProfiles: profiles } = useSettingsData(true);
  const currentProfileId = workspace?.default_config_agent_profile_id ?? "";
  const [saving, setSaving] = useState(false);
  const { toast } = useToast();
  const queryClient = useQueryClient();

  const handleChange = async (value: string) => {
    const effectiveValue = value === "none" ? "" : value;
    if (!workspace) return;
    setSaving(true);
    try {
      await updateWorkspaceAction(workspace.id, {
        default_config_agent_profile_id: effectiveValue,
      });
      patchWorkspaceCache(queryClient, workspace.id, {
        default_config_agent_profile_id: effectiveValue ? toAgentProfileId(effectiveValue) : null,
      });
      toast({ title: "Configuration agent updated", variant: "success" });
    } catch (error) {
      toast({
        title: "Failed to update",
        description: error instanceof Error ? error.message : "Unknown error",
        variant: "error",
      });
    } finally {
      setSaving(false);
    }
  };

  if (!workspace) return null;

  return (
    <div className="space-y-4">
      <Separator />
      <div>
        <h3 className="text-lg font-semibold">Configuration Chat Agent</h3>
        <p className="text-sm text-muted-foreground">
          Choose which agent profile to use for the Configuration Chat. This agent can manage your
          workflows, agent profiles, and MCP configuration.
        </p>
      </div>
      <Select value={currentProfileId || "none"} onValueChange={handleChange} disabled={saving}>
        <SelectTrigger className="w-full max-w-sm cursor-pointer">
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
    </div>
  );
}
