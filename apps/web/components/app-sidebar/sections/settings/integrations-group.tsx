"use client";

import {
  IconBrandGithub,
  IconBrandGitlab,
  IconBrandSlack,
  IconHexagon,
  IconPlugConnected,
  IconTicket,
} from "@tabler/icons-react";
import type { Icon as TablerIcon } from "@tabler/icons-react";
import { SettingsGroup, SettingsLeaf } from "./settings-nav-primitives";

const ROOT_HREF = "/settings/integrations";

const ITEMS: Array<{ href: string; label: string; icon: TablerIcon }> = [
  { href: `${ROOT_HREF}/github`, label: "GitHub", icon: IconBrandGithub },
  { href: `${ROOT_HREF}/gitlab`, label: "GitLab", icon: IconBrandGitlab },
  { href: `${ROOT_HREF}/jira`, label: "Jira", icon: IconTicket },
  { href: `${ROOT_HREF}/linear`, label: "Linear", icon: IconHexagon },
  { href: `${ROOT_HREF}/slack`, label: "Slack", icon: IconBrandSlack },
];

type IntegrationsGroupProps = {
  pathname: string;
  expanded?: boolean;
  onToggle?: () => void;
};

export function IntegrationsGroup({ pathname, expanded, onToggle }: IntegrationsGroupProps) {
  return (
    <SettingsGroup
      label="Integrations"
      icon={IconPlugConnected}
      href={ROOT_HREF}
      isActive={pathname === ROOT_HREF}
      expanded={expanded}
      onToggle={onToggle}
    >
      {ITEMS.map(({ href, label, icon }) => (
        <SettingsLeaf
          key={href}
          href={href}
          label={label}
          icon={icon}
          isActive={pathname === href}
          depth={1}
        />
      ))}
    </SettingsGroup>
  );
}
