"use client";

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  IconKey,
  IconMessageCircle,
  IconMicrophone,
  IconPlugConnected,
  IconPuzzle,
  IconWand,
} from "@tabler/icons-react";
import { PluginSlot } from "@/components/plugins/plugin-slot";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useSettingsDiscovery } from "@/hooks/domains/settings/use-settings-discovery";
import {
  EXTERNAL_MCP_SETTINGS_HREF,
  PLUGINS_SETTINGS_HREF,
  PROMPTS_SETTINGS_HREF,
  UTILITY_AGENTS_SETTINGS_HREF,
  VOICE_MODE_SETTINGS_HREF,
} from "@/lib/settings-discovery/catalog/standalone";
import { AccountGroup } from "./account-group";
import { AgentsGroup } from "./agents-group";
import { ExecutorsGroup } from "./executors-group";
import { GeneralGroup } from "./general-group";
import { SettingsLeaf } from "./settings-nav-primitives";
import { SettingsSearch } from "./settings-search";
import { SystemGroup } from "./system-group";
import { WorkspacesGroup } from "./workspaces-group";

const SECRETS_HREF = "/settings/general/secrets";
const INTEGRATIONS_SETTINGS_HREF = "/settings/integrations";
const DEFAULT_OPEN_GROUP = "workspaces";

// Single-open accordion: each top-level group owns a route prefix. The group
// whose prefix matches the current path is the open one. Prefixes are disjoint,
// so first match wins and ordering is irrelevant.
const GROUP_ROUTES = [
  { id: "general", prefix: "/settings/general" },
  { id: "workspaces", prefix: "/settings/workspace" },
  { id: "agents", prefix: "/settings/agents" },
  { id: "executors", prefix: "/settings/executors" },
  { id: "system", prefix: "/settings/system" },
  { id: "account", prefix: "/settings/account" },
] as const;

/** The settings accordion group that owns `pathname`, or null for a standalone leaf. */
export function settingsGroupIdForPath(pathname: string): string | null {
  return GROUP_ROUTES.find((g) => pathname.startsWith(g.prefix))?.id ?? null;
}

export function settingsOpenGroupIdForPath(pathname: string): string {
  return settingsGroupIdForPath(pathname) ?? DEFAULT_OPEN_GROUP;
}

/**
 * The settings nav tree. Top-level groups behave as a single-open accordion:
 * opening one closes the others and reveals its subsections. Routes without a
 * top-level group open Workspaces so the active workspace settings stay visible.
 *
 * Rendered both inside the collapsible "Settings" sidebar section and, when the
 * footer gear is active, as the full-height sidebar takeover.
 */
export function SettingsTree({ pathname }: { pathname: string }) {
  const { t } = useTranslation();
  const authEnabled = useFeature("auth");
  const authMode = useAppStore((s) => s.auth.mode);
  const showAccountGroup = authEnabled && authMode === "enabled";
  const discoveryItems = useSettingsDiscovery();
  const [query, setQuery] = useState("");
  const [openGroup, setOpenGroup] = useState<string | null>(() =>
    settingsOpenGroupIdForPath(pathname),
  );

  // Re-sync when navigation lands on a different section so the open group
  // always reflects the current page (a leaf with no owning group → all closed).
  useEffect(() => {
    setOpenGroup(settingsOpenGroupIdForPath(pathname));
  }, [pathname]);

  const groupProps = (id: string) => ({
    expanded: openGroup === id,
    onToggle: () => setOpenGroup((prev) => (prev === id ? null : id)),
  });

  return (
    <>
      <SettingsSearch
        items={discoveryItems}
        query={query}
        onQueryChange={setQuery}
        onSelect={() => setQuery("")}
      />
      {query.trim() ? null : (
        <>
          <GeneralGroup pathname={pathname} {...groupProps("general")} />
          <WorkspacesGroup pathname={pathname} {...groupProps("workspaces")} />
          <AgentsGroup pathname={pathname} {...groupProps("agents")} />
          <SettingsLeaf
            href={PROMPTS_SETTINGS_HREF}
            label={t("common:prompts")}
            icon={IconMessageCircle}
            isActive={pathname === PROMPTS_SETTINGS_HREF}
          />
          <SettingsLeaf
            href={VOICE_MODE_SETTINGS_HREF}
            label={t("settings:voiceMode")}
            icon={IconMicrophone}
            isActive={pathname === VOICE_MODE_SETTINGS_HREF}
          />
          <SettingsLeaf
            href={UTILITY_AGENTS_SETTINGS_HREF}
            label={t("settings:utilityAgents")}
            icon={IconWand}
            isActive={pathname === UTILITY_AGENTS_SETTINGS_HREF}
          />
          <ExecutorsGroup pathname={pathname} {...groupProps("executors")} />
          {/* Editors lives under General (see GeneralGroup) — no duplicate top-level leaf. */}
          <SettingsLeaf
            href={SECRETS_HREF}
            label={t("settings:secrets")}
            icon={IconKey}
            isActive={pathname === SECRETS_HREF}
          />
          <SettingsLeaf
            href={EXTERNAL_MCP_SETTINGS_HREF}
            label={t("common:externalMcp")}
            icon={IconPlugConnected}
            isActive={pathname === EXTERNAL_MCP_SETTINGS_HREF}
          />
          <SettingsLeaf
            href={INTEGRATIONS_SETTINGS_HREF}
            label={t("common:integrations")}
            icon={IconPlugConnected}
            isActive={pathname === INTEGRATIONS_SETTINGS_HREF}
          />
          <PluginSlot name="settings-nav" />
          <SettingsLeaf
            href={PLUGINS_SETTINGS_HREF}
            label={t("common:plugins")}
            icon={IconPuzzle}
            isActive={pathname === PLUGINS_SETTINGS_HREF}
          />
          <SystemGroup pathname={pathname} {...groupProps("system")} />
          {showAccountGroup && <AccountGroup pathname={pathname} {...groupProps("account")} />}
        </>
      )}
    </>
  );
}
