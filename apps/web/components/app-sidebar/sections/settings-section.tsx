"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  IconBolt,
  IconCode,
  IconCpu,
  IconFolder,
  IconKey,
  IconMessageCircle,
  IconPlugConnected,
  IconRobot,
  IconServerCog,
  IconSettings,
  IconWand,
} from "@tabler/icons-react";
import type { Icon as TablerIcon } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { APP_SIDEBAR_SECTION_IDS } from "../app-sidebar-constants";
import { AppSidebarSection } from "../app-sidebar-section";

type SettingsSectionProps = {
  collapsed: boolean;
};

type SettingsEntry = {
  label: string;
  href: string;
  icon: TablerIcon;
};

const SETTINGS_ENTRIES: SettingsEntry[] = [
  { label: "General", href: "/settings/general", icon: IconSettings },
  { label: "Workspaces", href: "/settings/workspace", icon: IconFolder },
  { label: "Integrations", href: "/settings/integrations", icon: IconPlugConnected },
  { label: "Automations", href: "/settings/automations", icon: IconBolt },
  { label: "Agents", href: "/settings/agents", icon: IconRobot },
  { label: "Prompts", href: "/settings/prompts", icon: IconMessageCircle },
  { label: "Utility Agents", href: "/settings/utility-agents", icon: IconWand },
  { label: "Executors", href: "/settings/executors", icon: IconCpu },
  { label: "Editors", href: "/settings/general/editors", icon: IconCode },
  { label: "Secrets", href: "/settings/general/secrets", icon: IconKey },
  { label: "External MCP", href: "/settings/external-mcp", icon: IconPlugConnected },
  { label: "System", href: "/settings/system/status", icon: IconServerCog },
];

export function SettingsSection({ collapsed }: SettingsSectionProps) {
  const pathname = usePathname();

  return (
    <AppSidebarSection
      id={APP_SIDEBAR_SECTION_IDS.settings}
      label="Settings"
      collapsed={collapsed}
      icon={IconSettings}
    >
      {SETTINGS_ENTRIES.map(({ label, href, icon: Icon }) => {
        const isActive = pathname === href || pathname.startsWith(`${href}/`);
        return (
          <Link
            key={href}
            href={href}
            className={cn(
              "flex items-center gap-2.5 px-2.5 py-1.5 text-[13px] font-medium rounded-md cursor-pointer",
              isActive ? "bg-accent text-foreground" : "text-foreground/80 hover:bg-muted/60",
            )}
          >
            <Icon className="h-4 w-4 shrink-0" />
            <span className="flex-1 truncate">{label}</span>
          </Link>
        );
      })}
    </AppSidebarSection>
  );
}
