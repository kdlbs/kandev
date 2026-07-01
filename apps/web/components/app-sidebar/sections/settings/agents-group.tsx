"use client";

import { IconRobot } from "@tabler/icons-react";
import { AgentLogo } from "@/components/agent-logo";
import { useAvailableAgents } from "@/hooks/domains/settings/use-available-agents";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { SettingsGroup, SettingsLeaf } from "./settings-nav-primitives";

const ROOT_HREF = "/settings/agents";

type AgentsGroupProps = {
  pathname: string;
  expanded?: boolean;
  onToggle?: () => void;
};

export function AgentsGroup({ pathname, expanded, onToggle }: AgentsGroupProps) {
  const { settingsAgents: agents } = useSettingsData(true);
  useAvailableAgents();

  return (
    <SettingsGroup
      label="Agents"
      icon={IconRobot}
      href={ROOT_HREF}
      isActive={pathname === ROOT_HREF}
      expanded={expanded}
      onToggle={onToggle}
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
