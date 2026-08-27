"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  listUtilityAgents,
  updateUtilityAgent,
  deleteUtilityAgent,
  type UtilityAgent,
} from "@/lib/api/domains/utility-api";
import { fetchUserSettings, updateUserSettings } from "@/lib/api/domains/settings-api";
import { SettingsPageHeader } from "@/components/settings/settings-typography";
import { Separator } from "@kandev/ui/separator";
import { UtilityAgentDialog } from "@/components/settings/utility-agent-dialog";
import { ConfigChatAgentSection } from "@/components/settings/config-chat-agent-section";
import {
  DefaultModelSection,
  PerActionOverridesSection,
  CustomAgentsSection,
  USE_DEFAULT,
} from "@/components/settings/utility-sections";
import { useAppStore } from "@/components/state-provider";
import { useSettingsSaveContributor } from "./settings-save-provider";
export { isUtilityAgentDirty } from "./utility-dirty";

export function replaceCustomUtilityAgents(
  current: UtilityAgent[],
  customAgents: UtilityAgent[],
): UtilityAgent[] {
  return [...current.filter((agent) => agent.builtin), ...customAgents];
}

export function mergeRefreshedUtilityAgents(
  current: UtilityAgent[],
  saved: UtilityAgent[],
  refreshed: UtilityAgent[],
): UtilityAgent[] {
  return refreshed.map((agent) => {
    if (!agent.builtin) return agent;
    const draft = current.find((item) => item.id === agent.id);
    const baseline = saved.find((item) => item.id === agent.id);
    if (!draft || !baseline) return agent;
    return {
      ...agent,
      agent_id: draft.agent_id !== baseline.agent_id ? draft.agent_id : agent.agent_id,
      model: draft.model !== baseline.model ? draft.model : agent.model,
      agent_profile_id:
        draft.agent_profile_id !== baseline.agent_profile_id
          ? draft.agent_profile_id
          : agent.agent_profile_id,
      profile_binding_state:
        draft.profile_binding_state !== baseline.profile_binding_state
          ? draft.profile_binding_state
          : agent.profile_binding_state,
      enabled: draft.enabled !== baseline.enabled ? draft.enabled : agent.enabled,
    };
  });
}

export function updateBuiltinProfileDraft(agent: UtilityAgent, value: string): UtilityAgent {
  const inherit = value === "" || value === USE_DEFAULT;
  return {
    ...agent,
    agent_profile_id: inherit ? "" : value,
    profile_binding_state: inherit ? "inherit" : "explicit",
    enabled: true,
  };
}

function updateBuiltinDraft(
  agent: UtilityAgent,
  value: string,
  setAgents: React.Dispatch<React.SetStateAction<UtilityAgent[]>>,
) {
  setAgents((prev) =>
    prev.map((item) => (item.id === agent.id ? updateBuiltinProfileDraft(item, value) : item)),
  );
}

function revision(agents: UtilityAgent[], profileId: string) {
  return JSON.stringify({
    profileId,
    builtins: agents
      .filter((agent) => agent.builtin)
      .map((agent) => ({
        id: agent.id,
        agent_profile_id: agent.agent_profile_id,
        profile_binding_state: agent.profile_binding_state,
        enabled: agent.enabled,
      })),
  });
}

// eslint-disable-next-line max-lines-per-function
export function UtilityAgentsSection() {
  const { t } = useTranslation();
  const profiles = useAppStore((state) => state.agentProfiles.items);
  const [agents, setAgents] = useState<UtilityAgent[]>([]);
  const [savedAgents, setSavedAgents] = useState<UtilityAgent[]>([]);
  const [defaultProfileId, setDefaultProfileId] = useState("");
  const [savedDefaultProfileId, setSavedDefaultProfileId] = useState("");
  const [loading, setLoading] = useState(true);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingAgent, setEditingAgent] = useState<UtilityAgent | null>(null);

  const fetchData = useCallback(async () => {
    try {
      const [agentsRes, settingsRes] = await Promise.all([
        listUtilityAgents({ cache: "no-store" }),
        fetchUserSettings({ cache: "no-store" }),
      ]);
      setAgents(agentsRes.agents);
      setSavedAgents(agentsRes.agents);
      const profileId = settingsRes.settings.default_utility_agent_profile_id || "";
      setDefaultProfileId(profileId);
      setSavedDefaultProfileId(profileId);
    } catch {
      setAgents([]);
      setSavedAgents([]);
    } finally {
      setLoading(false);
    }
  }, []);
  useEffect(() => {
    void fetchData();
  }, [fetchData]);

  const draftRevision = revision(agents, defaultProfileId);
  const savedRevision = revision(savedAgents, savedDefaultProfileId);
  useSettingsSaveContributor({
    id: "utility-agents",
    revision: draftRevision,
    isDirty: !loading && draftRevision !== savedRevision,
    save: async () => {
      const changed = agents.filter(
        (agent) =>
          agent.builtin &&
          (() => {
            const saved = savedAgents.find((item) => item.id === agent.id);
            return (
              !saved ||
              saved.agent_profile_id !== agent.agent_profile_id ||
              saved.profile_binding_state !== agent.profile_binding_state ||
              saved.enabled !== agent.enabled
            );
          })(),
      );
      await Promise.all([
        updateUserSettings({ default_utility_agent_profile_id: defaultProfileId }),
        ...changed.map((agent) =>
          updateUtilityAgent(agent.id, {
            agent_profile_id: agent.agent_profile_id,
            profile_binding_state: agent.profile_binding_state,
            enabled: agent.enabled,
          }),
        ),
      ]);
      setSavedAgents(agents);
      setSavedDefaultProfileId(defaultProfileId);
    },
    discard: () => {
      setAgents(savedAgents);
      setDefaultProfileId(savedDefaultProfileId);
    },
  });

  const builtins = useMemo(() => agents.filter((agent) => agent.builtin), [agents]);
  const customAgents = useMemo(() => agents.filter((agent) => !agent.builtin), [agents]);
  const closeDialog = () => {
    setDialogOpen(false);
    setEditingAgent(null);
    void listUtilityAgents({ cache: "no-store" }).then((response) => {
      setAgents((current) =>
        replaceCustomUtilityAgents(
          current,
          response.agents.filter((agent) => !agent.builtin),
        ),
      );
      setSavedAgents(response.agents);
    });
  };
  if (loading) return null;
  return (
    <div className="space-y-8">
      <SettingsPageHeader
        title={t("settings:utilityAgents")}
        description={t("settings:utilityAgentsPageDescription")}
      />
      <Separator />
      <div className="space-y-4">
        <DefaultModelSection
          profiles={profiles}
          profileId={defaultProfileId}
          onProfileChange={setDefaultProfileId}
          isDirty={defaultProfileId !== savedDefaultProfileId}
        />
        <ConfigChatAgentSection />
        <PerActionOverridesSection
          builtins={builtins}
          profiles={profiles}
          defaultLabel={
            defaultProfileId
              ? profiles.find((profile) => profile.id === defaultProfileId)?.label ||
                defaultProfileId
              : t("settings:default")
          }
          onProfileChange={(agent, value) => updateBuiltinDraft(agent, value, setAgents)}
          onEdit={(agent) => {
            setEditingAgent(agent);
            setDialogOpen(true);
          }}
          savedBuiltins={savedAgents.filter((agent) => agent.builtin)}
        />
        <CustomAgentsSection
          agents={customAgents}
          profiles={profiles}
          onAdd={() => {
            setEditingAgent(null);
            setDialogOpen(true);
          }}
          onEdit={(agent) => {
            setEditingAgent(agent);
            setDialogOpen(true);
          }}
          onDelete={async (agent) => {
            try {
              await deleteUtilityAgent(agent.id);
              setAgents((items) => items.filter((item) => item.id !== agent.id));
              setSavedAgents((items) => items.filter((item) => item.id !== agent.id));
            } catch (error) {
              console.error("Failed to delete utility agent", error);
            }
          }}
        />
      </div>
      <UtilityAgentDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        agent={editingAgent}
        onSuccess={closeDialog}
      />
    </div>
  );
}
