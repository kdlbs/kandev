"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { IconAlertTriangle, IconPlus, IconSettings } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
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
import { useAvailableAgents } from "@/hooks/domains/settings/use-available-agents";
import { AgentLogo } from "@/components/agent-logo";
import { AddTUIAgentDialog } from "@/components/settings/add-tui-agent-dialog";
import type { AgentDiscovery, Agent, AvailableAgent, AgentProfile } from "@/lib/types/http";

type AgentCardProps = {
  agent: AgentDiscovery;
  savedAgent: Agent | undefined;
  displayName: string;
};

function AgentCard({ agent, savedAgent, displayName }: AgentCardProps) {
  const configured = Boolean(savedAgent && savedAgent.profiles.length > 0);
  const hasAgentRecord = Boolean(savedAgent);
  return (
    <Card>
      <CardContent className="py-4 flex flex-col gap-3">
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            <AgentLogo agentName={agent.name} size={20} className="shrink-0" />
            <h4 className="font-medium">{displayName}</h4>
            {agent.supports_mcp && <Badge variant="secondary">MCP</Badge>}
            {configured && <Badge variant="outline">Configured</Badge>}
          </div>
          {agent.matched_path && (
            <p className="text-xs text-muted-foreground">Detected at {agent.matched_path}</p>
          )}
        </div>
        <Button size="sm" className="cursor-pointer" asChild>
          <Link
            href={
              hasAgentRecord
                ? `/settings/agents/${encodeURIComponent(agent.name)}?mode=create`
                : `/settings/agents/${encodeURIComponent(agent.name)}`
            }
          >
            <IconSettings className="h-4 w-4 mr-2" />
            {hasAgentRecord ? "Create new profile" : "Setup Profile"}
          </Link>
        </Button>
      </CardContent>
    </Card>
  );
}

type ProfileListItemProps = {
  agent: Agent;
  profile: AgentProfile;
};

function ProfileListItem({ agent, profile }: ProfileListItemProps) {
  const profilePath = `/settings/agents/${encodeURIComponent(agent.name)}/profiles/${profile.id}`;
  return (
    <Link href={profilePath} className="block">
      <Card className="hover:bg-accent transition-colors cursor-pointer">
        <CardContent className="py-2 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <AgentLogo agentName={agent.name} className="shrink-0" />
            <span className="text-sm font-medium">
              {agent.profiles[0]?.agent_display_name ?? agent.name}
            </span>
            {agent.supports_mcp && <Badge variant="secondary">MCP</Badge>}
            <span className="text-sm text-muted-foreground">{profile.name}</span>
          </div>
        </CardContent>
      </Card>
    </Link>
  );
}

function useAgentPageState() {
  const discoveryAgents = useAppStore((state) => state.agentDiscovery.items);
  const savedAgents = useAppStore((state) => state.settingsAgents.items);
  const setAgentDiscovery = useAppStore((state) => state.setAgentDiscovery);
  const setSettingsAgents = useAppStore((state) => state.setSettingsAgents);
  const setAvailableAgents = useAppStore((state) => state.setAvailableAgents);
  const availableAgents = useAvailableAgents().items;
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

  const handleRescan = async () => {
    if (rescanning) return;
    setRescanning(true);
    try {
      const response = await listAgentDiscovery({ cache: "no-store" });
      setAgentDiscovery(response.agents);
    } finally {
      setRescanning(false);
    }
  };

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
    setAvailableAgents(availableResp.agents);
  };

  return {
    savedAgents,
    installedAgents,
    savedAgentsByName,
    rescanning,
    tuiDialogOpen,
    setTuiDialogOpen,
    resolveDisplayName,
    handleRescan,
    handleCreateCustomTUI,
  };
}

export default function AgentsSettingsPage() {
  const {
    savedAgents,
    installedAgents,
    savedAgentsByName,
    rescanning,
    tuiDialogOpen,
    setTuiDialogOpen,
    resolveDisplayName,
    handleRescan,
    handleCreateCustomTUI,
  } = useAgentPageState();

  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold">Agents</h2>
        <p className="text-sm text-muted-foreground mt-1">
          Discover installed agents and continue setup from their configuration page.
        </p>
      </div>

      <Separator />

      <div className="space-y-4">
        <div className="flex items-start justify-between gap-4">
          <div>
            <h3 className="text-lg font-semibold">Supported agents found</h3>
            <p className="text-sm text-muted-foreground">
              Agents detected on this machine are ready to configure.
            </p>
          </div>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => setTuiDialogOpen(true)}
              className="cursor-pointer"
            >
              <IconPlus className="h-4 w-4 mr-2" />
              Add TUI Agent
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={handleRescan}
              disabled={rescanning}
              className="cursor-pointer"
            >
              {rescanning ? "Rescanning..." : "Rescan"}
            </Button>
          </div>
        </div>

        {installedAgents.length === 0 && (
          <Card>
            <CardContent className="py-8 text-center">
              <div className="flex items-center justify-center gap-2 text-sm text-muted-foreground">
                <IconAlertTriangle className="h-4 w-4" />
                No installed agents were detected yet.
              </div>
            </CardContent>
          </Card>
        )}

        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5">
          {installedAgents.map((agent: AgentDiscovery) => (
            <AgentCard
              key={agent.name}
              agent={agent}
              savedAgent={savedAgentsByName.get(agent.name)}
              displayName={resolveDisplayName(agent.name)}
            />
          ))}
        </div>
      </div>

      {savedAgents.some((agent: Agent) => agent.profiles.length > 0) && (
        <div className="space-y-4">
          <Separator />
          <div>
            <h3 className="text-lg font-semibold">Agent Profiles</h3>
            <p className="text-sm text-muted-foreground">Manage existing profiles by agent.</p>
          </div>

          <div className="space-y-2">
            {savedAgents.flatMap((agent: Agent) =>
              agent.profiles.map((profile: AgentProfile) => (
                <ProfileListItem key={profile.id} agent={agent} profile={profile} />
              )),
            )}
          </div>
        </div>
      )}

      <AddTUIAgentDialog
        open={tuiDialogOpen}
        onOpenChange={setTuiDialogOpen}
        onSubmit={handleCreateCustomTUI}
      />
    </div>
  );
}
