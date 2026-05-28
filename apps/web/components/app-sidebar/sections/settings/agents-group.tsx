"use client";

import { IconRobot } from "@tabler/icons-react";
import { AgentLogo } from "@/components/agent-logo";
import { useAppStore } from "@/components/state-provider";
import { useAvailableAgents } from "@/hooks/domains/settings/use-available-agents";
import { SettingsGroup, SettingsLeaf } from "./settings-nav-primitives";

const ROOT_HREF = "/settings/agents";

type AgentsGroupProps = {
  pathname: string;
};

export function AgentsGroup({ pathname }: AgentsGroupProps) {
  const agents = useAppStore((s) => s.settingsAgents.items);
  useAvailableAgents();
  const isAgents = pathname.startsWith(ROOT_HREF);

  return (
    <SettingsGroup
      label="Agents"
      icon={IconRobot}
      href={ROOT_HREF}
      isActive={pathname === ROOT_HREF}
      defaultExpanded={isAgents}
    >
      {agents.flatMap((agent) =>
        agent.profiles.map((profile) => {
          const encodedAgent = encodeURIComponent(agent.name);
          const profilePath = `${ROOT_HREF}/${encodedAgent}/profiles/${profile.id}`;
          const agentLabel = profile.agentDisplayName || agent.name;
          return (
            <SettingsLeaf
              key={profile.id}
              href={profilePath}
              label={`${agentLabel} • ${profile.name}`}
              leadingIcon={<AgentLogo agentName={agent.name} className="h-3.5 w-3.5 shrink-0" />}
              isActive={pathname === profilePath}
              depth={1}
            />
          );
        }),
      )}
    </SettingsGroup>
  );
}
