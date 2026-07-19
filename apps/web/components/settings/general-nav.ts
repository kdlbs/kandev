import {
  IconArchive,
  IconBell,
  IconCommand,
  IconCode,
  IconPalette,
  IconTerminal2,
} from "@tabler/icons-react";
import type { Icon as TablerIcon } from "@tabler/icons-react";

export type GeneralNavItem = {
  href: string;
  label: string;
  description: string;
  icon: TablerIcon;
};

export const GENERAL_NAV_ITEMS: GeneralNavItem[] = [
  {
    href: "/settings/general/appearance",
    label: "Appearance",
    description: "Theme, metrics, and changes panel preferences",
    icon: IconPalette,
  },
  {
    href: "/settings/general/terminal",
    label: "Terminal",
    description: "Shell, terminal fonts, and link behavior",
    icon: IconTerminal2,
  },
  {
    href: "/settings/general/notifications",
    label: "Notifications",
    description: "Providers and notification events",
    icon: IconBell,
  },
  {
    href: "/settings/general/editors",
    label: "Editors",
    description: "Editor integrations and defaults",
    icon: IconCode,
  },
  {
    href: "/settings/general/keyboard-shortcuts",
    label: "Keyboard Shortcuts",
    description: "Chat input and command shortcuts",
    icon: IconCommand,
  },
  {
    href: "/settings/general/task-actions",
    label: "Task Actions",
    description: "MCP task defaults and archive safeguards",
    icon: IconArchive,
  },
];
