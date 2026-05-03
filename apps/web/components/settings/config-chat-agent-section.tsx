"use client";

import { useState } from "react";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Separator } from "@kandev/ui/separator";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { updateWorkspaceAction } from "@/app/actions/workspaces";

export function ConfigChatAgentSection() {
  const workspace = useAppStore(
    (s) => s.workspaces.items.find((w) => w.id === s.workspaces.activeId) ?? null,
  );
  const profiles = useAppStore((s) => s.agentProfiles.items ?? []);
  const currentProfileId = workspace?.default_config_agent_profile_id ?? "";
  const [saving, setSaving] = useState(false);
  const { toast } = useToast();

  const storeApi = useAppStoreApi();

  const handleChange = async (value: string) => {
    const effectiveValue = value === "none" ? "" : value;
    if (!workspace) return;
    setSaving(true);
    try {
      await updateWorkspaceAction(workspace.id, {
        default_config_agent_profile_id: effectiveValue,
      });
      const { workspaces, setWorkspaces } = storeApi.getState();
      setWorkspaces(
        workspaces.items.map((w) =>
          w.id === workspace.id ? { ...w, default_config_agent_profile_id: effectiveValue } : w,
        ),
      );
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
