"use client";

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateWorkspaceAction } from "@/app/actions/workspaces";
import { useSettingsSaveContributor } from "./settings-save-provider";
import { SettingsCard } from "./settings-card";

export function ConfigChatAgentSection() {
  const { t } = useTranslation();
  const workspace = useAppStore(
    (s) => s.workspaces.items.find((w) => w.id === s.workspaces.activeId) ?? null,
  );
  const profiles = useAppStore((s) => s.agentProfiles.items ?? []);
  const currentProfileId = workspace?.default_config_agent_profile_id ?? "";
  const workspaceId = workspace?.id ?? null;
  const [syncedWorkspaceId, setSyncedWorkspaceId] = useState(workspaceId);
  const [savedProfileId, setSavedProfileId] = useState(currentProfileId);
  const [draftProfileId, setDraftProfileId] = useState(currentProfileId);

  const storeApi = useAppStoreApi();
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
      const { workspaces, setWorkspaces } = storeApi.getState();
      setWorkspaces(
        workspaces.items.map((w) =>
          w.id === workspace.id ? { ...w, default_config_agent_profile_id: submitted } : w,
        ),
      );
      setSavedProfileId(submitted);
    },
    discard: () => setDraftProfileId(savedProfileId),
  });

  if (!workspace) return null;

  return (
    <SettingsCard isDirty={isDirty} data-testid="config-chat-agent-card">
      <CardHeader>
        <CardTitle className="text-base">
          <h3>{t("settings:configChatAgentTitle")}</h3>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-sm text-muted-foreground">{t("settings:configChatAgentDescription")}</p>
        <Select
          value={draftProfileId || "none"}
          onValueChange={(value) => setDraftProfileId(value === "none" ? "" : value)}
        >
          <SelectTrigger className="w-full max-w-sm cursor-pointer" data-settings-dirty={isDirty}>
            <SelectValue placeholder={t("settings:configChatAgentPlaceholder")} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="none">{t("settings:configChatAgentNoDefault")}</SelectItem>
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
