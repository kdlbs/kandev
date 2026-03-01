"use client";

import { useEffect, useState, useCallback, useMemo } from "react";
import { Separator } from "@kandev/ui/separator";
import {
  listUtilityAgents,
  listInferenceAgents,
  updateUtilityAgent,
  deleteUtilityAgent,
  type UtilityAgent,
  type InferenceAgent,
} from "@/lib/api/domains/utility-api";
import { fetchUserSettings, updateUserSettings } from "@/lib/api/domains/settings-api";
import { UtilityAgentDialog } from "@/components/settings/utility-agent-dialog";
import {
  DefaultModelSection,
  PerActionOverridesSection,
  CustomAgentsSection,
  USE_DEFAULT,
} from "@/components/settings/utility-sections";

function buildAllModels(inferenceAgents: InferenceAgent[]) {
  return inferenceAgents.flatMap((ia) =>
    ia.models.map((m) => ({
      value: `${ia.id}|${m.id}`,
      label: `${ia.display_name} / ${m.name}`,
      agentName: ia.display_name,
      modelName: m.name,
    })),
  );
}

async function handleBuiltinChange(
  agent: UtilityAgent,
  value: string,
  setAgents: React.Dispatch<React.SetStateAction<UtilityAgent[]>>,
) {
  const isDefault = value === USE_DEFAULT;
  const [agentId, model] = isDefault ? ["", ""] : value.split("|");
  await updateUtilityAgent(agent.id, { agent_id: agentId, model, enabled: true });
  setAgents((prev) =>
    prev.map((a) => (a.id === agent.id ? { ...a, agent_id: agentId, model } : a)),
  );
}

export default function UtilityAgentsSettingsPage() {
  const [agents, setAgents] = useState<UtilityAgent[]>([]);
  const [inferenceAgents, setInferenceAgents] = useState<InferenceAgent[]>([]);
  const [defaultAgentId, setDefaultAgentId] = useState("");
  const [defaultModel, setDefaultModel] = useState("");
  const [loading, setLoading] = useState(true);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingAgent, setEditingAgent] = useState<UtilityAgent | null>(null);

  const builtins = useMemo(() => agents.filter((a) => a.builtin), [agents]);
  const customAgents = useMemo(() => agents.filter((a) => !a.builtin), [agents]);
  const allModels = useMemo(() => buildAllModels(inferenceAgents), [inferenceAgents]);

  const fetchData = useCallback(async () => {
    try {
      const [agentsRes, inferenceRes, settingsRes] = await Promise.all([
        listUtilityAgents({ cache: "no-store" }),
        listInferenceAgents(),
        fetchUserSettings({ cache: "no-store" }),
      ]);
      setAgents(agentsRes.agents);
      setInferenceAgents(inferenceRes.agents);
      setDefaultAgentId(settingsRes.settings.default_utility_agent_id || "");
      setDefaultModel(settingsRes.settings.default_utility_model || "");
    } catch {
      setAgents([]);
      setInferenceAgents([]);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  const handleDefaultChange = async (agentId: string, model: string) => {
    setDefaultAgentId(agentId);
    setDefaultModel(model);
    await updateUserSettings({ default_utility_agent_id: agentId, default_utility_model: model });
  };

  const openEditDialog = (agent: UtilityAgent | null) => {
    setEditingAgent(agent);
    setDialogOpen(true);
  };

  const closeDialog = () => {
    setDialogOpen(false);
    setEditingAgent(null);
    fetchData();
  };

  if (loading)
    return <div className="py-8 text-center text-muted-foreground text-sm">Loading...</div>;

  return (
    <div className="space-y-8">
      <header>
        <h2 className="text-xl font-semibold">Utility Agents</h2>
        <p className="text-sm text-muted-foreground">
          One-shot AI helpers for commits, PRs, and prompts.
        </p>
      </header>
      <Separator />
      <DefaultModelSection
        inferenceAgents={inferenceAgents}
        defaultAgentId={defaultAgentId}
        defaultModel={defaultModel}
        onDefaultChange={handleDefaultChange}
      />
      <PerActionOverridesSection
        builtins={builtins}
        allModels={allModels}
        defaultModel={defaultModel}
        onModelChange={(agent, value) => handleBuiltinChange(agent, value, setAgents)}
        onEdit={openEditDialog}
      />
      <CustomAgentsSection
        agents={customAgents}
        onAdd={() => openEditDialog(null)}
        onEdit={openEditDialog}
        onDelete={async (agent) => {
          await deleteUtilityAgent(agent.id);
          setAgents((prev) => prev.filter((a) => a.id !== agent.id));
        }}
      />
      <UtilityAgentDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        agent={editingAgent}
        onSuccess={closeDialog}
      />
    </div>
  );
}
