"use client";

import { IconRobot } from "@tabler/icons-react";
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
              isActive={pathname === profilePath}
              depth={1}
            />
          );
        }),
      )}
    </SettingsGroup>
  );
}
