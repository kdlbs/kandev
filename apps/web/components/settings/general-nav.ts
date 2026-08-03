import { useTranslation } from "react-i18next";
import {
  IconArchive,
  IconBell,
  IconCommand,
  IconCode,
  IconLayoutDashboard,
  IconMessage,
  IconPalette,
  IconTerminal2,
} from "@tabler/icons-react";
import type { Icon as TablerIcon } from "@tabler/icons-react";

export type GeneralNavItem = {
  href: string;
  labelKey: string;
  descriptionKey: string;
  icon: TablerIcon;
};

export const GENERAL_NAV_ITEMS: GeneralNavItem[] = [
  {
    href: "/settings/general/appearance",
    labelKey: "settings:appearance",
    descriptionKey: "settings:themeMetricsAndChangesPanelPreferences",
    icon: IconPalette,
  },
  {
    href: "/settings/general/layouts",
    labelKey: "settings:layouts",
    descriptionKey: "settings:taskWorkbenchLayoutProfilesAndDefaults",
    icon: IconLayoutDashboard,
  },
  {
    href: "/settings/general/terminal",
    labelKey: "settings:terminal",
    descriptionKey: "settings:shellTerminalFontsAndLinkBehavior",
    icon: IconTerminal2,
  },
  {
    href: "/settings/general/notifications",
    labelKey: "settings:notifications",
    descriptionKey: "settings:providersAndNotificationEvents",
    icon: IconBell,
  },
  {
    href: "/settings/general/editors",
    labelKey: "settings:editors",
    descriptionKey: "settings:editorIntegrationsAndDefaults",
    icon: IconCode,
  },
  {
    href: "/settings/general/keyboard-shortcuts",
    labelKey: "settings:keyboardShortcuts",
    descriptionKey: "settings:chatInputAndCommandShortcuts",
    icon: IconCommand,
  },
  {
    href: "/settings/general/task-actions",
    labelKey: "settings:taskActions",
    descriptionKey: "settings:mcpTaskDefaultsAndArchiveSafeguards",
    icon: IconArchive,
  },
  {
    href: "/settings/general/message-queue",
    labelKey: "system:messageQueueTitle",
    descriptionKey: "system:messageQueueDescription",
    icon: IconMessage,
  },
];

/** A nav item with its copy already translated, ready to render. */
export type ResolvedGeneralNavItem = {
  href: string;
  label: string;
  description: string;
  icon: TablerIcon;
};

/**
 * Translate {@link GENERAL_NAV_ITEMS} at render time. The base list is a
 * module-level constant evaluated once at import, so it holds catalog KEYS —
 * calling `t()` in the const itself would pin the copy to the boot locale.
 */
export function useGeneralNavItems(): ResolvedGeneralNavItem[] {
  const { t } = useTranslation();
  return GENERAL_NAV_ITEMS.map((item) => ({
    href: item.href,
    icon: item.icon,
    label: t(item.labelKey),
    description: t(item.descriptionKey),
  }));
}
