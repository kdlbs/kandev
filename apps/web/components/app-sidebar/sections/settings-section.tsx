"use client";

import { usePathname } from "next/navigation";
import {
  IconBolt,
  IconCode,
  IconKey,
  IconMessageCircle,
  IconPlugConnected,
  IconSettings,
  IconWand,
} from "@tabler/icons-react";
import { APP_SIDEBAR_SECTION_IDS } from "../app-sidebar-constants";
import { AppSidebarSection } from "../app-sidebar-section";
import { AgentsGroup } from "./settings/agents-group";
import { ExecutorsGroup } from "./settings/executors-group";
import { GeneralGroup } from "./settings/general-group";
import { IntegrationsGroup } from "./settings/integrations-group";
import { SettingsLeaf } from "./settings/settings-nav-primitives";
import { SystemGroup } from "./settings/system-group";
import { WorkspacesGroup } from "./settings/workspaces-group";

type SettingsSectionProps = {
  collapsed: boolean;
};

const EDITORS_HREF = "/settings/general/editors";
const SECRETS_HREF = "/settings/general/secrets";
const AUTOMATIONS_HREF = "/settings/automations";
const PROMPTS_HREF = "/settings/prompts";
const UTILITY_HREF = "/settings/utility-agents";
const EXT_MCP_HREF = "/settings/external-mcp";

export function SettingsSection({ collapsed }: SettingsSectionProps) {
  const pathname = usePathname();

  return (
    <AppSidebarSection
      id={APP_SIDEBAR_SECTION_IDS.settings}
      label="Settings"
      collapsed={collapsed}
      icon={IconSettings}
    >
      <GeneralGroup pathname={pathname} />
      <WorkspacesGroup pathname={pathname} />
      <IntegrationsGroup pathname={pathname} />
      <SettingsLeaf
        href={AUTOMATIONS_HREF}
        label="Automations"
        icon={IconBolt}
        isActive={pathname.startsWith(AUTOMATIONS_HREF)}
      />
      <AgentsGroup pathname={pathname} />
      <SettingsLeaf
        href={PROMPTS_HREF}
        label="Prompts"
        icon={IconMessageCircle}
        isActive={pathname === PROMPTS_HREF}
      />
      <SettingsLeaf
        href={UTILITY_HREF}
        label="Utility Agents"
        icon={IconWand}
        isActive={pathname === UTILITY_HREF}
      />
      <ExecutorsGroup pathname={pathname} />
      <SettingsLeaf
        href={EDITORS_HREF}
        label="Editors"
        icon={IconCode}
        isActive={pathname === EDITORS_HREF}
      />
      <SettingsLeaf
        href={SECRETS_HREF}
        label="Secrets"
        icon={IconKey}
        isActive={pathname === SECRETS_HREF}
      />
      <SettingsLeaf
        href={EXT_MCP_HREF}
        label="External MCP"
        icon={IconPlugConnected}
        isActive={pathname === EXT_MCP_HREF}
      />
      <SystemGroup pathname={pathname} />
    </AppSidebarSection>
  );
}
