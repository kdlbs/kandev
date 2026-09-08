"use client";

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { settingsActionClassName } from "@/components/settings/settings-control";
import Link from "@/components/routing/app-link";
import {
  IconAlertTriangle,
  IconDownload,
  IconLoader2,
  IconPlus,
  IconRefresh,
  IconTerminal2,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { Separator } from "@kandev/ui/separator";
import { useAppStore } from "@/components/state-provider";
import {
  createCustomTUIAgent,
  listAgentDiscovery,
  listAgents,
  listAvailableAgents,
} from "@/lib/api";
import { useIsAdmin } from "@/hooks/domains/auth/use-is-admin";
import type { AgentUpdateJob, AgentUpdatePreview, AgentUpdateStatus, InstallJob } from "@/lib/api";
import { useAgentDiscovery } from "@/hooks/domains/settings/use-agent-discovery";
import { useAgentRuntimeUpdates } from "@/hooks/domains/settings/use-agent-runtime-updates";
import { useAgentRuntimeUpdateStatuses } from "@/hooks/domains/settings/use-agent-runtime-update-statuses";
import { useAvailableAgents } from "@/hooks/domains/settings/use-available-agents";
import { AddTUIAgentDialog } from "@/components/settings/add-tui-agent-dialog";
import { AgentProfilesSubList } from "@/components/settings/agents/agent-profiles-section";
import { HostShellDialog } from "@/components/settings/host-shell-dialog";
import { CustomTUIMcpCard } from "@/components/settings/custom-tui-mcp-card";
import { InstalledAgentCard } from "@/components/settings/installed-agent-card";
import { DynamicAgentsCard } from "@/components/settings/dynamic-agents-card";
import { AGENTS_BROWSE_SETTINGS_HREF } from "@/lib/settings-discovery/catalog/agents";
import {
  detectedAgents,
  DYNAMIC_AGENT_NAME,
  orderAgentsForDisplay,
  orphanedAgents,
  type DiscoveredAgent,
} from "@/lib/settings/agent-display-order";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";
import { HideDisabledAgentProfilesSetting } from "@/app/settings/agents/hide-disabled-agent-profiles-setting";
import type { AgentDiscovery, Agent, AvailableAgent, RuntimeUpdate } from "@/lib/types/http";

type InstalledAgentsSectionProps = {
  installedAgents: AgentDiscovery[];
  /** The full scan, detected or not — the backend's ranking of every agent. */
  discoveryOrder: AgentDiscovery[];
  savedAgents: Agent[];
  discoveryLoading: boolean;
  rescanning: boolean;
  savedAgentsByName: Map<string, Agent>;
  resolveDisplayName: (name: string) => string;
  resolveCapabilityStatus: (name: string) => string | undefined;
  resolveRuntimeUpdate: (name: string) => RuntimeUpdate | undefined;
  resolveRuntimeUpdateStatus: (name: string) => AgentUpdateStatus | undefined;
  installJobs: Record<string, InstallJob>;
  updateJobs: Record<string, AgentUpdateJob>;
  previewUpdate: (
    name: string,
    targetVersion?: string,
    useDefault?: boolean,
  ) => Promise<AgentUpdatePreview>;
  startUpdate: (
    name: string,
    targetVersion: string,
    useDefault?: boolean,
  ) => Promise<AgentUpdateJob>;
  setTuiDialogOpen: (open: boolean) => void;
  handleRescan: () => Promise<void>;
  canManage: boolean;
};

function InstalledAgentsHeader({
  rescanning,
  onOpenShell,
  onOpenTuiDialog,
  onRescan,
}: {
  rescanning: boolean;
  onOpenShell: () => void;
  /** Absent for a caller without org.config.manage: creating a TUI agent is
   *  a write this page must not offer them. Rescan and the host shell stay:
   *  discovery is a read. */
  onOpenTuiDialog?: () => void;
  onRescan: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
      <div>
        <h3 className="text-lg font-semibold">{t("agents:installedAgents")}</h3>
        <p className="text-sm text-muted-foreground">{t("agents:installedAgentsDescription")}</p>
      </div>
      <div className="flex w-full flex-wrap gap-2 md:w-auto" data-testid="installed-agents-actions">
        <Button
          variant="outline"
          size="sm"
          onClick={onOpenShell}
          className="h-11 cursor-pointer md:h-6"
          data-testid="open-host-shell"
        >
          <IconTerminal2 className="h-4 w-4 mr-2" />
          {t("agents:terminal")}
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={onRescan}
          disabled={rescanning}
          className="h-11 cursor-pointer md:h-6"
          data-testid="rescan-agents-button"
        >
          {rescanning ? (
            <IconLoader2 className="h-4 w-4 mr-2 animate-spin" />
          ) : (
            <IconRefresh className="h-4 w-4 mr-2" />
          )}
          {t("agents:rescan")}
        </Button>
        {onOpenTuiDialog && (
          <Button
            variant="outline"
            size="sm"
            onClick={onOpenTuiDialog}
            className="h-11 cursor-pointer md:h-6"
            data-testid="new-agent-button"
          >
            <IconPlus className="h-4 w-4 mr-2" />
            {t("agents:addTuiAgent")}
          </Button>
        )}
      </div>
    </div>
  );
}

type AgentCard = { key: string; agent: AgentDiscovery; detected: boolean };

/**
 * Every agent worth a card, in the backend's rank order: the ones the scan
 * found, plus configured agents it did not, the latter given a synthetic
 * discovery record so a missing CLI never hides its profiles.
 */
function agentCards(
  installedAgents: AgentDiscovery[],
  savedAgents: Agent[],
  discoveryOrder: DiscoveredAgent[],
): AgentCard[] {
  const detected: AgentCard[] = installedAgents.map((agent) => ({
    key: agent.name,
    agent,
    detected: true,
  }));
  const orphans: AgentCard[] = orphanedAgents(installedAgents, savedAgents)
    .filter((agent) => agent.name !== DYNAMIC_AGENT_NAME)
    .map((agent) => ({
      key: agent.id,
      agent: {
        name: agent.name,
        supports_mcp: agent.supports_mcp,
        mcp_config_path: agent.mcp_config_path ?? null,
        installation_paths: [],
        available: false,
        matched_path: null,
      },
      detected: false,
    }));
  // Rank by name against the same list the menu ranks against, so the two
  // surfaces cannot order the same agents differently.
  const ordered = orderAgentsForDisplay(
    discoveryOrder,
    [...detected, ...orphans].map((card) => ({ name: card.agent.name, profiles: [], card })),
  );
  return ordered.map((entry) => entry.card);
}

function InstalledAgentsSection({
  installedAgents,
  discoveryOrder,
  savedAgents,
  discoveryLoading,
  rescanning,
  savedAgentsByName,
  resolveDisplayName,
  resolveCapabilityStatus,
  resolveRuntimeUpdate,
  resolveRuntimeUpdateStatus,
  installJobs,
  updateJobs,
  previewUpdate,
  startUpdate,
  setTuiDialogOpen,
  handleRescan,
  canManage,
}: InstalledAgentsSectionProps) {
  const { t } = useTranslation();
  const [shellOpen, setShellOpen] = useState(false);

  // One ranked list rather than "detected, then the rest". Two groups meant an
  // agent the scan misses always sorted below every detected one — which put
  // the dev-only mock agent, ranked last by the backend, ahead of real agents
  // whose CLI happened to be absent. Undetected agents still render, via a
  // synthetic discovery record, so their profiles never vanish; they just keep
  // their rank. The settings menu ranks the same way — see `agent-display-order`.
  const cards = agentCards(installedAgents, savedAgents, discoveryOrder);
  const dynamicAgent = savedAgentsByName.get(DYNAMIC_AGENT_NAME);

  return (
    <div className="space-y-4">
      <InstalledAgentsHeader
        rescanning={rescanning}
        onOpenShell={() => setShellOpen(true)}
        onOpenTuiDialog={canManage ? () => setTuiDialogOpen(true) : undefined}
        onRescan={() => void handleRescan()}
      />
      <HostShellDialog
        open={shellOpen}
        onOpenChange={setShellOpen}
        onClose={() => {
          // Rescan after the user closes the shell - they may have installed
          // an agent CLI or fixed an auth issue inside the terminal.
          void handleRescan();
        }}
      />

      <DynamicAgentsCard agent={dynamicAgent} />

      {cards.length === 0 && (
        <Card>
          <CardContent className="py-8 text-center">
            <div className="flex items-center justify-center gap-2 text-sm text-muted-foreground">
              {discoveryLoading ? (
                <span>{t("agents:scanningForInstalledAgents")}</span>
              ) : (
                <>
                  <IconAlertTriangle className="h-4 w-4" />
                  {t("agents:noInstalledAgentsDetected")}
                </>
              )}
            </div>
          </CardContent>
        </Card>
      )}

      <div className="grid gap-3">
        {cards.map(({ key, agent, detected }) => (
          <InstalledAgentCard
            key={key}
            agent={agent}
            savedAgent={savedAgentsByName.get(agent.name)}
            displayName={resolveDisplayName(agent.name)}
            {...(detected
              ? {
                  capabilityStatus: resolveCapabilityStatus(agent.name),
                  runtimeUpdate: resolveRuntimeUpdate(agent.name),
                  runtimeUpdateStatus: resolveRuntimeUpdateStatus(agent.name),
                  installJob: installJobs[agent.name],
                  updateJob: updateJobs[agent.name],
                  onPreview: canManage ? previewUpdate : undefined,
                  onUpdate: canManage ? startUpdate : undefined,
                  onAuthComplete: () => void handleRescan(),
                }
              : {})}
          >
            <AgentProfilesSubList
              savedAgent={savedAgentsByName.get(agent.name)}
              agentName={agent.name}
            />
            <CustomTUIMcpCard agent={savedAgentsByName.get(agent.name)} />
          </InstalledAgentCard>
        ))}
      </div>
    </div>
  );
}

function useAgentPageState() {
  const { items: discoveryAgents, loading: discoveryLoading } = useAgentDiscovery();
  const savedAgents = useAppStore((state) => state.settingsAgents.items);
  const setAgentDiscovery = useAppStore((state) => state.setAgentDiscovery);
  const setSettingsAgents = useAppStore((state) => state.setSettingsAgents);
  const setAvailableAgents = useAppStore((state) => state.setAvailableAgents);
  const setAgentProfiles = useAppStore((state) => state.setAgentProfiles);
  const installJobs = useAppStore((state) => state.installJobs.byAgent);
  const { items: availableAgents } = useAvailableAgents();
  const [rescanning, setRescanning] = useState(false);
  const [tuiDialogOpen, setTuiDialogOpen] = useState(false);
  // Agents and agent profiles are org configuration: every write behind this
  // page requires org.config.manage, which only an administrator holds.
  const canManage = useIsAdmin();
  const { updateJobs, previewUpdate, startUpdate } = useAgentRuntimeUpdates();
  const { refresh: refreshRuntimeUpdateStatuses, statusByAgent } =
    useAgentRuntimeUpdateStatuses(updateJobs);

  const installedAgents = useMemo(() => detectedAgents(discoveryAgents), [discoveryAgents]);
  const savedAgentsByName = useMemo(
    () => new Map(savedAgents.map((agent: Agent) => [agent.name, agent])),
    [savedAgents],
  );
  const resolveDisplayName = (name: string) =>
    availableAgents.find((item: AvailableAgent) => item.name === name)?.display_name ?? name;
  const resolveCapabilityStatus = (name: string) =>
    availableAgents.find((item: AvailableAgent) => item.name === name)?.model_config?.status;
  const resolveRuntimeUpdate = (name: string) =>
    availableAgents.find((item: AvailableAgent) => item.name === name)?.runtime_update;
  const resolveRuntimeUpdateStatus = (name: string) => statusByAgent[name];

  const handleRescan = async () => {
    if (rescanning) {
      return;
    }
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
  }) => {
    await createCustomTUIAgent(data);
    const [discoveryResp, agentsResp, availableResp] = await Promise.all([
      listAgentDiscovery({ cache: "no-store" }),
      listAgents({ cache: "no-store" }),
      listAvailableAgents({ cache: "no-store" }),
    ]);
    setAgentDiscovery(discoveryResp.agents);
    setSettingsAgents(agentsResp.agents);
    setAgentProfiles(
      agentsResp.agents.flatMap((agent) =>
        agent.profiles.map((profile) => toAgentProfileOption(agent, profile)),
      ),
    );
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
    resolveDisplayName,
    resolveCapabilityStatus,
    resolveRuntimeUpdate,
    resolveRuntimeUpdateStatus,
    handleRescan,
    handleCreateCustomTUI,
    installJobs,
    updateJobs,
    previewUpdate,
    startUpdate,
  };
}

export default function AgentsSettingsPage() {
  const {
    canManage,
    savedAgents,
    installedAgents,
    discoveryAgents,
    savedAgentsByName,
    discoveryLoading,
    rescanning,
    tuiDialogOpen,
    setTuiDialogOpen,
    resolveDisplayName,
    resolveCapabilityStatus,
    resolveRuntimeUpdate,
    resolveRuntimeUpdateStatus,
    handleRescan,
    handleCreateCustomTUI,
    installJobs,
    updateJobs,
    previewUpdate,
    startUpdate,
  } = useAgentPageState();
  const { t } = useTranslation();

  return (
    <div className="space-y-8">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h2 className="text-2xl font-bold">{t("common:agents")}</h2>
          <p className="text-sm text-muted-foreground mt-1">{t("agents:pageDescription")}</p>
        </div>
        {/* The page's primary action: everything else on this page manages what
            is already installed. */}
        {canManage && (
          <Button
            size="sm"
            className={settingsActionClassName("cursor-pointer")}
            asChild
            data-testid="install-agents-button"
          >
            <Link href={AGENTS_BROWSE_SETTINGS_HREF}>
              <IconDownload className="h-4 w-4 mr-2" />
              {t("agents:installAgents")}
            </Link>
          </Button>
        )}
      </div>

      <Separator />

      <HideDisabledAgentProfilesSetting />

      <InstalledAgentsSection
        canManage={canManage}
        installedAgents={installedAgents}
        discoveryOrder={discoveryAgents}
        savedAgents={savedAgents}
        discoveryLoading={discoveryLoading}
        rescanning={rescanning}
        savedAgentsByName={savedAgentsByName}
        resolveDisplayName={resolveDisplayName}
        resolveCapabilityStatus={resolveCapabilityStatus}
        resolveRuntimeUpdate={resolveRuntimeUpdate}
        resolveRuntimeUpdateStatus={resolveRuntimeUpdateStatus}
        installJobs={installJobs}
        updateJobs={updateJobs}
        previewUpdate={previewUpdate}
        startUpdate={startUpdate}
        setTuiDialogOpen={setTuiDialogOpen}
        handleRescan={handleRescan}
      />

      <AddTUIAgentDialog
        open={tuiDialogOpen}
        onOpenChange={setTuiDialogOpen}
        onSubmit={handleCreateCustomTUI}
      />
    </div>
  );
}
