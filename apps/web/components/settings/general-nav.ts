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
import { GENERAL_DISCOVERY_DEFINITIONS } from "@/lib/settings-discovery/catalog/general";

export type GeneralNavItem = {
  href: string;
  labelKey: string;
  descriptionKey: string;
  icon: TablerIcon;
};

const GENERAL_NAV_PRESENTATION: Array<
  Pick<GeneralNavItem, "descriptionKey" | "icon"> & { id: string }
> = [
  {
    id: "general-appearance",
    descriptionKey: "settings:themeMetricsAndChangesPanelPreferences",
    icon: IconPalette,
  },
  {
    id: "general-layouts",
    descriptionKey: "settings:taskWorkbenchLayoutProfilesAndDefaults",
    icon: IconLayoutDashboard,
  },
  {
    id: "general-terminal",
    descriptionKey: "settings:shellTerminalFontsAndLinkBehavior",
    icon: IconTerminal2,
  },
  {
    id: "general-notifications",
    descriptionKey: "settings:providersAndNotificationEvents",
    icon: IconBell,
  },
  {
    id: "general-editors",
    descriptionKey: "settings:editorIntegrationsAndDefaults",
    icon: IconCode,
  },
  {
    id: "general-keyboard-shortcuts",
    descriptionKey: "settings:chatInputAndCommandShortcuts",
    icon: IconCommand,
  },
  {
    id: "general-task-actions",
    descriptionKey: "settings:mcpTaskDefaultsAndArchiveSafeguards",
    icon: IconArchive,
  },
  {
    id: "general-message-queue",
    descriptionKey: "system:messageQueueDescription",
    icon: IconMessage,
  },
];

const generalPages = new Map(GENERAL_DISCOVERY_DEFINITIONS.map((item) => [item.id, item]));

export const GENERAL_NAV_ITEMS: GeneralNavItem[] = GENERAL_NAV_PRESENTATION.map((item) => {
  const page = generalPages.get(item.id);
  if (!page?.labelKey) throw new Error(`Missing General discovery page: ${item.id}`);
  return {
    href: page.href,
    labelKey: page.labelKey,
    descriptionKey: item.descriptionKey,
    icon: item.icon,
  };
});

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
