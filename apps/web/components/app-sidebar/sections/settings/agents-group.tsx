"use client";

import { useTranslation } from "react-i18next";
import { IconRobot } from "@tabler/icons-react";
import { AgentLogo } from "@/components/agent-logo";
import { useAppStore } from "@/components/state-provider";
import { useAvailableAgents } from "@/hooks/domains/settings/use-available-agents";
import { AGENTS_SETTINGS_HREF } from "@/lib/settings-discovery/catalog/agents";
import { SettingsGroup, SettingsLeaf } from "./settings-nav-primitives";

type AgentsGroupProps = {
  pathname: string;
  expanded?: boolean;
  onToggle?: () => void;
};

export function AgentsGroup({ pathname, expanded, onToggle }: AgentsGroupProps) {
  const { t } = useTranslation();
  const agents = useAppStore((s) => s.settingsAgents.items);
  useAvailableAgents();

  return (
    <SettingsGroup
      label={t("common:agents")}
      icon={IconRobot}
      href={AGENTS_SETTINGS_HREF}
      isActive={pathname === AGENTS_SETTINGS_HREF}
      expanded={expanded}
      onToggle={onToggle}
    >
      {agents.flatMap((agent) =>
        agent.profiles.map((profile) => {
          const encodedAgent = encodeURIComponent(agent.name);
          const profilePath = `${AGENTS_SETTINGS_HREF}/${encodedAgent}/profiles/${profile.id}`;
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
