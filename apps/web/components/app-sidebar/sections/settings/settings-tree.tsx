"use client";

import { useEffect, useState } from "react";
import {
  IconBolt,
  IconKey,
  IconMessageCircle,
  IconPlugConnected,
  IconWand,
} from "@tabler/icons-react";
import { AgentsGroup } from "./agents-group";
import { ExecutorsGroup } from "./executors-group";
import { GeneralGroup } from "./general-group";
import { IntegrationsGroup } from "./integrations-group";
import { SettingsLeaf } from "./settings-nav-primitives";
import { SystemGroup } from "./system-group";
import { WorkspacesGroup } from "./workspaces-group";

const AUTOMATIONS_HREF = "/settings/automations";
const PROMPTS_HREF = "/settings/prompts";
const UTILITY_HREF = "/settings/utility-agents";
const SECRETS_HREF = "/settings/general/secrets";
const EXT_MCP_HREF = "/settings/external-mcp";

// Single-open accordion: each top-level group owns a route prefix. The group
// whose prefix matches the current path is the open one. Prefixes are disjoint,
// so first match wins and ordering is irrelevant.
const GROUP_ROUTES = [
  { id: "general", prefix: "/settings/general" },
  { id: "workspaces", prefix: "/settings/workspace" },
  { id: "integrations", prefix: "/settings/integrations" },
  { id: "agents", prefix: "/settings/agents" },
  { id: "executors", prefix: "/settings/executors" },
  { id: "system", prefix: "/settings/system" },
] as const;

/** The settings accordion group that owns `pathname`, or null for a standalone leaf. */
export function settingsGroupIdForPath(pathname: string): string | null {
  return GROUP_ROUTES.find((g) => pathname.startsWith(g.prefix))?.id ?? null;
}

/**
 * The settings nav tree. Top-level groups behave as a single-open accordion:
 * opening one closes the others and reveals its subsections. Navigating to a
 * standalone leaf (Prompts, Automations, …) closes every group.
 *
 * Rendered both inside the collapsible "Settings" sidebar section and, when the
 * footer gear is active, as the full-height sidebar takeover.
 */
export function SettingsTree({ pathname }: { pathname: string }) {
  const [openGroup, setOpenGroup] = useState<string | null>(() => settingsGroupIdForPath(pathname));

  // Re-sync when navigation lands on a different section so the open group
  // always reflects the current page (a leaf with no owning group → all closed).
  useEffect(() => {
    setOpenGroup(settingsGroupIdForPath(pathname));
  }, [pathname]);

  const groupProps = (id: string) => ({
    expanded: openGroup === id,
    onToggle: () => setOpenGroup((prev) => (prev === id ? null : id)),
  });

  return (
    <>
      <GeneralGroup pathname={pathname} {...groupProps("general")} />
      <WorkspacesGroup pathname={pathname} {...groupProps("workspaces")} />
      <IntegrationsGroup pathname={pathname} {...groupProps("integrations")} />
      <SettingsLeaf
        href={AUTOMATIONS_HREF}
        label="Automations"
        icon={IconBolt}
        isActive={pathname.startsWith(AUTOMATIONS_HREF)}
      />
      <AgentsGroup pathname={pathname} {...groupProps("agents")} />
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
      <ExecutorsGroup pathname={pathname} {...groupProps("executors")} />
      {/* Editors lives under General (see GeneralGroup) — no duplicate top-level leaf. */}
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
      <SystemGroup pathname={pathname} {...groupProps("system")} />
    </>
  );
}
