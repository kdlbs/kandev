"use client";

import {
  IconActivity,
  IconArchive,
  IconDatabase,
  IconFileText,
  IconFlask,
  IconBrandGithub,
  IconInfoCircle,
  IconRefresh,
  IconScale,
  IconServerCog,
  IconTrash,
} from "@tabler/icons-react";
import type { Icon as TablerIcon } from "@tabler/icons-react";
import { SettingsGroup, SettingsLeaf } from "./settings-nav-primitives";

const ROOT_HREF = "/settings/system";
const DEFAULT_HREF = `${ROOT_HREF}/status`;

const ITEMS: Array<{ href: string; label: string; icon: TablerIcon }> = [
  { href: `${ROOT_HREF}/status`, label: "Status", icon: IconActivity },
  { href: `${ROOT_HREF}/feature-toggles`, label: "Feature Toggles", icon: IconFlask },
  { href: `${ROOT_HREF}/github-app`, label: "GitHub App", icon: IconBrandGithub },
  { href: `${ROOT_HREF}/database`, label: "Database", icon: IconDatabase },
  { href: `${ROOT_HREF}/backups`, label: "Backups", icon: IconArchive },
  { href: `${ROOT_HREF}/storage`, label: "Storage", icon: IconTrash },
  { href: `${ROOT_HREF}/logs`, label: "Logs", icon: IconFileText },
  { href: `${ROOT_HREF}/updates`, label: "Updates", icon: IconRefresh },
  { href: `${ROOT_HREF}/about`, label: "About", icon: IconInfoCircle },
  { href: `${ROOT_HREF}/licenses`, label: "Licenses", icon: IconScale },
];

type SystemGroupProps = {
  pathname: string;
  expanded?: boolean;
  onToggle?: () => void;
};

export function SystemGroup({ pathname, expanded, onToggle }: SystemGroupProps) {
  return (
    <SettingsGroup
      label="System"
      icon={IconServerCog}
      href={DEFAULT_HREF}
      isActive={pathname.startsWith(ROOT_HREF)}
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
