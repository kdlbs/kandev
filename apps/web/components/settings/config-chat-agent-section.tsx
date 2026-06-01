"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Separator } from "@kandev/ui/separator";
import { useAppStore } from "@/components/state-provider";
import { useAgentProfiles } from "@/hooks/domains/settings/use-settings-reads";
import { useToast } from "@/components/toast-provider";
import { updateWorkspaceAction } from "@/app/actions/workspaces";
import { useWorkspace } from "@/hooks/domains/workspace/use-workspaces";
import { qk } from "@/lib/query/keys";

export function ConfigChatAgentSection() {
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  const workspace = useWorkspace(activeWorkspaceId);
  const profiles = useAgentProfiles();
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
      // The workspace.updated WS event also writes the TQ cache; invalidate
      // here so the picker reflects the change immediately.
      await queryClient.invalidateQueries({ queryKey: qk.workspaces.all() });
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
