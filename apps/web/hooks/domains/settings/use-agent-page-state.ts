"use client";

import { useMemo, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  createCustomTUIAgent,
  listAgentDiscovery,
  listAgents,
  listAvailableAgents,
} from "@/lib/api";
import { useIsAdmin } from "@/hooks/domains/auth/use-is-admin";
import { useAgentDiscovery } from "@/hooks/domains/settings/use-agent-discovery";
import { useAgentRuntimeUpdates } from "@/hooks/domains/settings/use-agent-runtime-updates";
import { useAgentRuntimeUpdateStatuses } from "@/hooks/domains/settings/use-agent-runtime-update-statuses";
import { useAvailableAgents } from "@/hooks/domains/settings/use-available-agents";
import { canInstallAgent } from "@/components/settings/install-agent-card";
import { AGENTS_BROWSE_SETTINGS_HREF } from "@/lib/settings-discovery/catalog/agents";
import { detectedAgents } from "@/lib/settings/agent-display-order";
import type { Agent, AvailableAgent } from "@/lib/types/http";
import type { AgentUpdateStatus } from "@/lib/api";
import { getAgentListResourceScope } from "@/hooks/domains/settings/agent-list-resource";

function createAgentSettingsResolvers(
  availableAgents: AvailableAgent[],
  statusByAgent: Record<string, AgentUpdateStatus | undefined>,
) {
  const agentsByName = new Map(availableAgents.map((agent) => [agent.name, agent]));
  const findAgent = (name: string) => agentsByName.get(name);
  return {
    resolveDisplayName: (name: string) => findAgent(name)?.display_name ?? name,
    resolveCapabilityStatus: (name: string) => findAgent(name)?.model_config?.status,
    resolveLoginCommand: (name: string) => findAgent(name)?.login_command,
    resolveRuntimeVariant: (name: string) => findAgent(name)?.model_config?.runtime_variant,
    resolveRuntimeUpdate: (name: string) => findAgent(name)?.runtime_update,
    resolveRuntimeUpdateStatus: (name: string) => statusByAgent[name],
  };
}

function openCodeReinstallHref({
  canManage,
  hasAvailableDiscovery,
  savedAgent,
  availableAgent,
  runtimeStatus,
}: {
  canManage: boolean;
  hasAvailableDiscovery: boolean;
  savedAgent: Agent | undefined;
  availableAgent: AvailableAgent | undefined;
  runtimeStatus: AgentUpdateStatus | undefined;
}): string | undefined {
  if (
    !canManage ||
    hasAvailableDiscovery ||
    !savedAgent?.profiles.length ||
    !availableAgent ||
    availableAgent.available ||
    !canInstallAgent(availableAgent) ||
    runtimeStatus?.runtime_source !== "managed" ||
    runtimeStatus.owner !== "kandev" ||
    runtimeStatus.management !== "managed"
  ) {
    return undefined;
  }
  return `${AGENTS_BROWSE_SETTINGS_HREF}?agent=opencode-acp`;
}

export function useAgentPageState() {
  const { items: discoveryAgents, loading: discoveryLoading } = useAgentDiscovery();
  const savedAgents = useAppStore((state) => state.settingsAgents.items);
  const setAgentDiscovery = useAppStore((state) => state.setAgentDiscovery);
  const setAvailableAgents = useAppStore((state) => state.setAvailableAgents);
  const storeApi = useAppStoreApi();
  const installJobs = useAppStore((state) => state.installJobs.byAgent);
  const { items: availableAgents } = useAvailableAgents();
  const [rescanning, setRescanning] = useState(false);
  const [tuiDialogOpen, setTuiDialogOpen] = useState(false);
  // Agent settings writes require org.config.manage, which only an administrator holds.
  const canManage = useIsAdmin();
  const { updateJobs, previewUpdate, startUpdate } = useAgentRuntimeUpdates();
  const {
    refresh: refreshRuntimeUpdateStatuses,
    statusByAgent,
    loading: runtimeStatusLoading,
    refreshFailed: runtimeStatusFailed,
  } = useAgentRuntimeUpdateStatuses(updateJobs);

  const handleStartUpdate = async (...args: Parameters<typeof startUpdate>) => {
    const result = await startUpdate(...args);
    if (!result.job_id && result.operation === "up_to_date") {
      void refreshRuntimeUpdateStatuses().catch(() => {});
    }
    return result;
  };

  const installedAgents = useMemo(() => detectedAgents(discoveryAgents), [discoveryAgents]);
  const savedAgentsByName = useMemo(
    () => new Map(savedAgents.map((agent: Agent) => [agent.name, agent])),
    [savedAgents],
  );
  const agentResolvers = createAgentSettingsResolvers(availableAgents, statusByAgent);
  const openCodeAvailableInDiscovery = discoveryAgents.some(
    (agent) => agent.name === "opencode-acp" && agent.available,
  );
  const reinstallOpenCodeHref = openCodeReinstallHref({
    canManage,
    hasAvailableDiscovery: openCodeAvailableInDiscovery,
    savedAgent: savedAgentsByName.get("opencode-acp"),
    availableAgent: availableAgents.find((agent) => agent.name === "opencode-acp"),
    runtimeStatus: statusByAgent["opencode-acp"],
  });

  const handleRescan = async () => {
    if (rescanning) return;
    setRescanning(true);
    try {
      const [discoveryResp, availableResp] = await Promise.all([
        listAgentDiscovery({ cache: "no-store" }),
        listAvailableAgents({ cache: "no-store" }),
      ]);
      setAgentDiscovery(discoveryResp.agents);
      setAvailableAgents(availableResp.agents, availableResp.tools ?? []);
      await refreshRuntimeUpdateStatuses();
    } finally {
      setRescanning(false);
    }
  };

  const handleCreateCustomTUI = async (data: {
    display_name: string;
    model?: string;
    command: string;
    mcp_strategy?: string;
    protocol?: string;
  }) => {
    await createCustomTUIAgent(data);
    const profileVersion = storeApi.getState().agentProfiles.version;
    const [discoveryResp, agentsResp, availableResp] = await Promise.all([
      listAgentDiscovery({ cache: "no-store" }),
      listAgents({ cache: "no-store" }),
      listAvailableAgents({ cache: "no-store" }),
    ]);
    setAgentDiscovery(discoveryResp.agents);
    if (!storeApi.getState().applyAgentListSnapshot(agentsResp.agents, profileVersion)) {
      await getAgentListResourceScope(storeApi).ensure();
    }
    setAvailableAgents(availableResp.agents, availableResp.tools ?? []);
  };

  return {
    canManage,
    savedAgents,
    installedAgents,
    discoveryAgents,
    savedAgentsByName,
    discoveryLoading,
    rescanning,
    tuiDialogOpen,
    setTuiDialogOpen,
    ...agentResolvers,
    reinstallOpenCodeHref,
    statusByAgent,
    runtimeStatusLoading,
    runtimeStatusFailed,
    refreshRuntimeUpdateStatuses,
    handleRescan,
    handleCreateCustomTUI,
    installJobs,
    updateJobs,
    previewUpdate,
    startUpdate: handleStartUpdate,
  };
}
