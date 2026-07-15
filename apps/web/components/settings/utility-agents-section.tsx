"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { IconWand } from "@tabler/icons-react";
import {
  listUtilityAgents,
  listInferenceAgents,
  updateUtilityAgent,
  deleteUtilityAgent,
  type UtilityAgent,
  type InferenceAgent,
} from "@/lib/api/domains/utility-api";
import { fetchUserSettings, updateUserSettings } from "@/lib/api/domains/settings-api";
import { SettingsSection } from "@/components/settings/settings-section";
import { UtilityAgentDialog } from "@/components/settings/utility-agent-dialog";
import {
  DefaultModelSection,
  PerActionOverridesSection,
  CustomAgentsSection,
  USE_DEFAULT,
} from "@/components/settings/utility-sections";
import { useInferenceAgents } from "@/components/settings/use-inference-agents";
import { useSettingsSaveContributor } from "./settings-save-provider";

function buildAllModels(inferenceAgents: InferenceAgent[]) {
  return inferenceAgents.flatMap((ia) =>
    (ia.models ?? []).map((m) => ({
      value: `${ia.id}|${m.id}`,
      label: `${ia.display_name} / ${m.name}`,
      agentName: ia.display_name,
      modelName: m.name,
    })),
  );
}

export function replaceCustomUtilityAgents(
  current: UtilityAgent[],
  customAgents: UtilityAgent[],
): UtilityAgent[] {
  return [...current.filter((agent) => agent.builtin), ...customAgents];
}

function updateBuiltinDraft(
  agent: UtilityAgent,
  value: string,
  setAgents: React.Dispatch<React.SetStateAction<UtilityAgent[]>>,
) {
  const isDefault = value === USE_DEFAULT;
  const [agentId, model] = isDefault ? ["", ""] : value.split("|");
  setAgents((prev) =>
    prev.map((a) => (a.id === agent.id ? { ...a, agent_id: agentId, model, enabled: true } : a)),
  );
}

type UtilityDraftRegistration = {
  agents: UtilityAgent[];
  savedAgents: UtilityAgent[];
  defaultAgentId: string;
  defaultModel: string;
  savedDefault: { agentId: string; model: string };
  loading: boolean;
  setAgents: React.Dispatch<React.SetStateAction<UtilityAgent[]>>;
  setSavedAgents: React.Dispatch<React.SetStateAction<UtilityAgent[]>>;
  setDefaultAgentId: React.Dispatch<React.SetStateAction<string>>;
  setDefaultModel: React.Dispatch<React.SetStateAction<string>>;
  setSavedDefault: React.Dispatch<React.SetStateAction<{ agentId: string; model: string }>>;
};

function utilityDraftRevision(
  agents: UtilityAgent[],
  defaultAgentId: string,
  defaultModel: string,
) {
  return JSON.stringify({
    defaultAgentId,
    defaultModel,
    builtins: agents
      .filter((agent) => agent.builtin)
      .map(({ id, agent_id, model, enabled }) => ({ id, agent_id, model, enabled })),
  });
}

function useUtilityDraftRegistration(state: UtilityDraftRegistration) {
  const draftRevision = utilityDraftRevision(
    state.agents,
    state.defaultAgentId,
    state.defaultModel,
  );
  const savedRevision = utilityDraftRevision(
    state.savedAgents,
    state.savedDefault.agentId,
    state.savedDefault.model,
  );

  useSettingsSaveContributor({
    id: "utility-agents",
    revision: draftRevision,
    isDirty: !state.loading && draftRevision !== savedRevision,
    save: async () => {
      const submittedAgents = state.agents;
      const submittedDefault = { agentId: state.defaultAgentId, model: state.defaultModel };
      const changedBuiltins = submittedAgents.filter((agent) => {
        if (!agent.builtin) return false;
        const previous = state.savedAgents.find((saved) => saved.id === agent.id);
        return (
          !previous ||
          previous.agent_id !== agent.agent_id ||
          previous.model !== agent.model ||
          previous.enabled !== agent.enabled
        );
      });
      await Promise.all([
        updateUserSettings({
          default_utility_agent_id: submittedDefault.agentId,
          default_utility_model: submittedDefault.model,
        }),
        ...changedBuiltins.map((agent) =>
          updateUtilityAgent(agent.id, {
            agent_id: agent.agent_id,
            model: agent.model,
            enabled: agent.enabled,
          }),
        ),
      ]);
      state.setSavedDefault(submittedDefault);
      state.setSavedAgents(submittedAgents);
    },
    discard: () => {
      state.setDefaultAgentId(state.savedDefault.agentId);
      state.setDefaultModel(state.savedDefault.model);
      state.setAgents(state.savedAgents);
    },
  });
}

function useUtilityAgentsData() {
  const [agents, setAgents] = useState<UtilityAgent[]>([]);
  const [savedAgents, setSavedAgents] = useState<UtilityAgent[]>([]);
  const { inferenceAgents, setInferenceAgents, refreshAgent } = useInferenceAgents();
  const [defaultAgentId, setDefaultAgentId] = useState("");
  const [defaultModel, setDefaultModel] = useState("");
  const [savedDefault, setSavedDefault] = useState({ agentId: "", model: "" });
  const [loading, setLoading] = useState(true);

  const fetchData = useCallback(async () => {
    try {
      const [agentsRes, inferenceRes, settingsRes] = await Promise.all([
        listUtilityAgents({ cache: "no-store" }),
        listInferenceAgents(),
        fetchUserSettings({ cache: "no-store" }),
      ]);
      const defaultDraft = {
        agentId: settingsRes.settings.default_utility_agent_id || "",
        model: settingsRes.settings.default_utility_model || "",
      };
      setAgents(agentsRes.agents);
      setSavedAgents(agentsRes.agents);
      setInferenceAgents(inferenceRes.agents);
      setDefaultAgentId(defaultDraft.agentId);
      setDefaultModel(defaultDraft.model);
      setSavedDefault(defaultDraft);
    } catch {
      setAgents([]);
      setInferenceAgents([]);
    } finally {
      setLoading(false);
    }
  }, [setInferenceAgents]);

  useEffect(() => {
    void fetchData();
  }, [fetchData]);

  const refreshCustomAgents = useCallback(async () => {
    const response = await listUtilityAgents({ cache: "no-store" });
    const customAgents = response.agents.filter((agent) => !agent.builtin);
    setAgents((current) => replaceCustomUtilityAgents(current, customAgents));
    setSavedAgents((current) => replaceCustomUtilityAgents(current, customAgents));
  }, []);

  return {
    agents,
    savedAgents,
    inferenceAgents,
    defaultAgentId,
    defaultModel,
    savedDefault,
    loading,
    refreshAgent,
    refreshCustomAgents,
    setAgents,
    setSavedAgents,
    setDefaultAgentId,
    setDefaultModel,
    setSavedDefault,
  };
}

export function UtilityAgentsSection() {
  const data = useUtilityAgentsData();
  const {
    agents,
    savedAgents,
    inferenceAgents,
    defaultAgentId,
    defaultModel,
    savedDefault,
    loading,
    refreshAgent,
    refreshCustomAgents,
    setAgents,
    setSavedAgents,
    setDefaultAgentId,
    setDefaultModel,
    setSavedDefault,
  } = data;
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingAgent, setEditingAgent] = useState<UtilityAgent | null>(null);

  const builtins = useMemo(() => agents.filter((a) => a.builtin), [agents]);
  const customAgents = useMemo(() => agents.filter((a) => !a.builtin), [agents]);
  const allModels = useMemo(() => buildAllModels(inferenceAgents), [inferenceAgents]);

  const handleDefaultChange = (agentId: string, model: string) => {
    setDefaultAgentId(agentId);
    setDefaultModel(model);
  };

  useUtilityDraftRegistration({
    agents,
    savedAgents,
    defaultAgentId,
    defaultModel,
    savedDefault,
    loading,
    setAgents,
    setSavedAgents,
    setDefaultAgentId,
    setDefaultModel,
    setSavedDefault,
  });

  const openEditDialog = (agent: UtilityAgent | null) => {
    setEditingAgent(agent);
    setDialogOpen(true);
  };

  const closeDialog = () => {
    setDialogOpen(false);
    setEditingAgent(null);
    void refreshCustomAgents();
  };

  if (loading) return null;

  return (
    <>
      <SettingsSection
        icon={<IconWand className="h-5 w-5" />}
        title="Utility Agents"
        description="One-shot AI helpers for commits, PRs, and prompts."
      >
        <div className="space-y-4">
          <DefaultModelSection
            inferenceAgents={inferenceAgents}
            defaultAgentId={defaultAgentId}
            defaultModel={defaultModel}
            onDefaultChange={handleDefaultChange}
            onRefreshAgent={refreshAgent}
          />
          <PerActionOverridesSection
            builtins={builtins}
            allModels={allModels}
            defaultModel={defaultModel}
            onModelChange={(agent, value) => updateBuiltinDraft(agent, value, setAgents)}
            onEdit={openEditDialog}
          />
          <CustomAgentsSection
            agents={customAgents}
            onAdd={() => openEditDialog(null)}
            onEdit={openEditDialog}
            onDelete={async (agent) => {
              try {
                await deleteUtilityAgent(agent.id);
                setAgents((prev) => prev.filter((a) => a.id !== agent.id));
                setSavedAgents((prev) => prev.filter((a) => a.id !== agent.id));
              } catch {
                // Error already logged by API layer
              }
            }}
          />
        </div>
      </SettingsSection>
      <UtilityAgentDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        agent={editingAgent}
        onSuccess={closeDialog}
      />
    </>
  );
}
