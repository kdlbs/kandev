'use client';

import { useMemo, useState } from 'react';
import Link from 'next/link';
import { IconAlertTriangle, IconSettings } from '@tabler/icons-react';
import { Badge } from '@kandev/ui/badge';
import { Button } from '@kandev/ui/button';
import { Card, CardContent } from '@kandev/ui/card';
import { Separator } from '@kandev/ui/separator';
import { useAppStore } from '@/components/state-provider';
import { listAgentDiscovery } from '@/lib/http';
import { useAvailableAgents } from '@/hooks/use-available-agents';

export default function AgentsSettingsPage() {
  const discoveryAgents = useAppStore((state) => state.agentDiscovery.items);
  const savedAgents = useAppStore((state) => state.settingsAgents.items);
  const setAgentDiscovery = useAppStore((state) => state.setAgentDiscovery);
  const availableAgents = useAvailableAgents().items;
  const [rescanning, setRescanning] = useState(false);

  const installedAgents = useMemo(
    () => discoveryAgents.filter((agent) => agent.available),
    [discoveryAgents]
  );

  const savedAgentsByName = useMemo(
    () => new Map(savedAgents.map((agent) => [agent.name, agent])),
    [savedAgents]
  );

  const handleRescan = async () => {
    if (rescanning) return;
    setRescanning(true);
    try {
      const response = await listAgentDiscovery({ cache: 'no-store' });
      setAgentDiscovery(response.agents);
    } finally {
      setRescanning(false);
    }
  };

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
          <Button variant="outline" size="sm" onClick={handleRescan} disabled={rescanning}>
            {rescanning ? 'Rescanning…' : 'Rescan'}
          </Button>
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
          {installedAgents.map((agent) => {
            const savedAgent = savedAgentsByName.get(agent.name);
            const configured = Boolean(savedAgent && savedAgent.profiles.length > 0);
            const hasAgentRecord = Boolean(savedAgent);
            return (
              <Card key={agent.name}>
                <CardContent className="py-4 flex flex-col gap-3">
                  <div className="space-y-1">
                    <div className="flex items-center gap-2">
                      <h4 className="font-medium">
                        {availableAgents.find((item) => item.name === agent.name)?.display_name ?? agent.name}
                      </h4>
                      {agent.supports_mcp && <Badge variant="secondary">MCP</Badge>}
                      {configured && <Badge variant="outline">Configured</Badge>}
                    </div>
                    {agent.matched_path && (
                      <p className="text-xs text-muted-foreground">Detected at {agent.matched_path}</p>
                    )}
                  </div>
                  <Button size="sm" className="cursor-pointer" asChild>
                    <Link href={`/settings/agents/${encodeURIComponent(agent.name)}`}>
                      <IconSettings className="h-4 w-4 mr-2" />
                      {hasAgentRecord ? 'Create new profile' : 'Setup Profile'}
                    </Link>
                  </Button>
                </CardContent>
              </Card>
            );
          })}
        </div>
      </div>

      {savedAgents.some((agent) => agent.profiles.length > 0) && (
        <div className="space-y-4">
          <Separator />
          <div>
            <h3 className="text-lg font-semibold">Agent Profiles</h3>
            <p className="text-sm text-muted-foreground">Manage existing profiles by agent.</p>
          </div>

          <div className="space-y-2">
            {savedAgents.flatMap((agent) =>
              agent.profiles.map((profile) => {
                const profilePath = `/settings/agents/${encodeURIComponent(agent.name)}/profiles/${profile.id}`;
                return (
                  <Link key={profile.id} href={profilePath} className="block">
                    <Card className="hover:bg-accent transition-colors cursor-pointer">
                      <CardContent className="py-2 flex items-center justify-between">
                        <div className="flex items-center gap-2">
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
              })
            )}
          </div>
        </div>
      )}
    </div>
  );
}
