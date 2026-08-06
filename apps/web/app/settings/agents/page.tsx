"use client";

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
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
import { createCustomTUIAgent, listAgentDiscovery, listAgents, listAvailableAgents } from "@/lib/api";
import type { AgentUpdateJob, AgentUpdatePreview, InstallJob } from "@/lib/api";
import { useAgentDiscovery } from "@/hooks/domains/settings/use-agent-discovery";
import { useAgentRuntimeUpdates } from "@/hooks/domains/settings/use-agent-runtime-updates";
import { useAvailableAgents } from "@/hooks/domains/settings/use-available-agents";
import { AddTUIAgentDialog } from "@/components/settings/add-tui-agent-dialog";
import { AgentProfilesSubList } from "@/components/settings/agents/agent-profiles-section";
import { HostShellDialog } from "@/components/settings/host-shell-dialog";
import { InstalledAgentCard } from "@/components/settings/installed-agent-card";
import { AGENTS_BROWSE_SETTINGS_HREF } from "@/lib/settings-discovery/catalog/agents";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";
import type {
  AgentDiscovery,
  Agent,
  AvailableAgent,
  RuntimeUpdate,
} from "@/lib/types/http";

type InstalledAgentsSectionProps = {
  installedAgents: AgentDiscovery[];
  savedAgents: Agent[];
  discoveryLoading: boolean;
  rescanning: boolean;
  savedAgentsByName: Map<string, Agent>;
  resolveDisplayName: (name: string) => string;
  resolveCapabilityStatus: (name: string) => string | undefined;
  resolveRuntimeUpdate: (name: string) => RuntimeUpdate | undefined;
  installJobs: Record<string, InstallJob>;
  updateJobs: Record<string, AgentUpdateJob>;
  previewUpdate: (name: string) => Promise<AgentUpdatePreview>;
  startUpdate: (name: string) => Promise<AgentUpdateJob>;
  setTuiDialogOpen: (open: boolean) => void;
  handleRescan: () => Promise<void>;
};

function InstalledAgentsHeader({
  rescanning,
  onOpenShell,
  onOpenTuiDialog,
  onRescan,
}: {
  rescanning: boolean;
  onOpenShell: () => void;
  onOpenTuiDialog: () => void;
  onRescan: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
      <div>
        <h3 className="text-lg font-semibold">{t("agents:installedAgents")}</h3>
        <p className="text-sm text-muted-foreground">{t("agents:installedAgentsDescription")}</p>
      </div>
      <div className="flex w-full flex-wrap gap-2 sm:w-auto">
        <Button
          variant="outline"
          size="sm"
          onClick={onOpenShell}
          className="cursor-pointer"
          data-testid="open-host-shell"
        >
          <IconTerminal2 className="h-4 w-4 mr-2" />
          {t("agents:terminal")}
        </Button>
        <Button variant="outline" size="sm" onClick={onOpenTuiDialog} className="cursor-pointer">
          <IconPlus className="h-4 w-4 mr-2" />
          {t("agents:addTuiAgent")}
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={onRescan}
          disabled={rescanning}
          className="cursor-pointer"
        >
          {rescanning ? (
            <IconLoader2 className="h-4 w-4 mr-2 animate-spin" />
          ) : (
            <IconRefresh className="h-4 w-4 mr-2" />
          )}
          {t("agents:rescan")}
        </Button>
      </div>
    </div>
  );
}

function InstalledAgentsSection({
  installedAgents,
  savedAgents,
  discoveryLoading,
  rescanning,
  savedAgentsByName,
  resolveDisplayName,
  resolveCapabilityStatus,
  resolveRuntimeUpdate,
  installJobs,
  updateJobs,
  previewUpdate,
  startUpdate,
  setTuiDialogOpen,
  handleRescan,
}: InstalledAgentsSectionProps) {
  const { t } = useTranslation();
  const [shellOpen, setShellOpen] = useState(false);

  // Configured agents whose CLI the scan no longer detects still get a group
  // (via a synthetic discovery record), so their profiles never vanish.
  const installedNames = new Set(installedAgents.map((agent) => agent.name));
  const orphanAgents = savedAgents.filter(
    (agent) => agent.profiles.length > 0 && !installedNames.has(agent.name),
  );

  return (
    <div className="space-y-4">
      <InstalledAgentsHeader
        rescanning={rescanning}
        onOpenShell={() => setShellOpen(true)}
        onOpenTuiDialog={() => setTuiDialogOpen(true)}
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

      {installedAgents.length === 0 && orphanAgents.length === 0 && (
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
        {installedAgents.map((agent: AgentDiscovery) => (
          <InstalledAgentCard
            key={agent.name}
            agent={agent}
            savedAgent={savedAgentsByName.get(agent.name)}
            displayName={resolveDisplayName(agent.name)}
            capabilityStatus={resolveCapabilityStatus(agent.name)}
            runtimeUpdate={resolveRuntimeUpdate(agent.name)}
            installJob={installJobs[agent.name]}
            updateJob={updateJobs[agent.name]}
            onPreview={previewUpdate}
            onUpdate={startUpdate}
            onAuthComplete={() => void handleRescan()}
          >
            <AgentProfilesSubList
              savedAgent={savedAgentsByName.get(agent.name)}
              agentName={agent.name}
            />
          </InstalledAgentCard>
        ))}
        {orphanAgents.map((agent: Agent) => (
          <InstalledAgentCard
            key={agent.id}
            agent={{
              name: agent.name,
              supports_mcp: agent.supports_mcp,
              mcp_config_path: agent.mcp_config_path ?? null,
              installation_paths: [],
              available: false,
              matched_path: null,
            }}
            savedAgent={agent}
            displayName={resolveDisplayName(agent.name)}
          >
            <AgentProfilesSubList savedAgent={agent} agentName={agent.name} />
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

  const installedAgents = useMemo(
    () => discoveryAgents.filter((agent: AgentDiscovery) => agent.available),
    [discoveryAgents],
  );
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
    } finally {
      setRescanning(false);
    }
  };

  const { updateJobs, previewUpdate, startUpdate } = useAgentRuntimeUpdates();

  const handleCreateCustomTUI = async (data: {
    display_name: string;
    model?: string;
    command: string;
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
    savedAgents,
    installedAgents,
    savedAgentsByName,
    discoveryLoading,
    rescanning,
    tuiDialogOpen,
    setTuiDialogOpen,
    resolveDisplayName,
    resolveCapabilityStatus,
    resolveRuntimeUpdate,
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
    savedAgents,
    installedAgents,
    savedAgentsByName,
    discoveryLoading,
    rescanning,
    tuiDialogOpen,
    setTuiDialogOpen,
    resolveDisplayName,
    resolveCapabilityStatus,
    resolveRuntimeUpdate,
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
        <Button size="sm" className="cursor-pointer" asChild data-testid="install-agents-button">
          <Link href={AGENTS_BROWSE_SETTINGS_HREF}>
            <IconDownload className="h-4 w-4 mr-2" />
            {t("agents:installAgents")}
          </Link>
        </Button>
      </div>

      <Separator />

      <InstalledAgentsSection
        installedAgents={installedAgents}
        savedAgents={savedAgents}
        discoveryLoading={discoveryLoading}
        rescanning={rescanning}
        savedAgentsByName={savedAgentsByName}
        resolveDisplayName={resolveDisplayName}
        resolveCapabilityStatus={resolveCapabilityStatus}
        resolveRuntimeUpdate={resolveRuntimeUpdate}
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
