"use client";

import {
  IconBell,
  IconCommand,
  IconCode,
  IconPalette,
  IconSettings,
  IconTerminal2,
} from "@tabler/icons-react";
import type { Icon as TablerIcon } from "@tabler/icons-react";
import { SettingsGroup, SettingsLeaf } from "./settings-nav-primitives";

const GENERAL_HREF = "/settings/general";

const GENERAL_ITEMS: Array<{ href: string; label: string; icon: TablerIcon }> = [
  { href: "/settings/general/appearance", label: "Appearance", icon: IconPalette },
  { href: "/settings/general/terminal", label: "Terminal", icon: IconTerminal2 },
  { href: "/settings/general/notifications", label: "Notifications", icon: IconBell },
  { href: "/settings/general/editors", label: "Editors", icon: IconCode },
  {
    href: "/settings/general/keyboard-shortcuts",
    label: "Keyboard Shortcuts",
    icon: IconCommand,
  },
];

type GeneralGroupProps = {
  pathname: string;
  expanded?: boolean;
  onToggle?: () => void;
};

export function GeneralGroup({ pathname, expanded, onToggle }: GeneralGroupProps) {
  return (
    <SettingsGroup
      label="General"
      icon={IconSettings}
      href={GENERAL_HREF}
      isActive={pathname === GENERAL_HREF}
      expanded={expanded}
      onToggle={onToggle}
    >
      {GENERAL_ITEMS.map(({ href, label, icon }) => (
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
