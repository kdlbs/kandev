"use client";

import { createContext, useContext, type ReactNode } from "react";

import { IntegrationEnabledBadge } from "@/components/settings/record-badges";
import {
  useEnabledIntegrations,
  type IntegrationSlug,
} from "@/hooks/domains/integrations/use-enabled-integrations";
import { usePluginRegistry } from "@/lib/plugins/registry";

const NONE: ReadonlySet<string> = new Set();

const EnabledIntegrationsContext = createContext<ReadonlySet<string>>(NONE);

/**
 * Which workspace's Integrations branch the rows below belong to. Fed by
 * {@link IntegrationsEnabledProvider} so plugin-integration badge rows can
 * resolve their per-workspace enabled state without threading a prop through
 * the generic menu node tree.
 */
const BadgeWorkspaceContext = createContext<string | null>(null);

/**
 * Probes one workspace's integrations once and shares the answer with its rows.
 *
 * Per branch rather than per row: `useIntegrationAuthed` fetches from its own
 * effect with no shared cache, so a hook call in each of the six integration
 * rows would be six times the requests for one answer.
 *
 * Mounted inside the branch's collapsible content, which Radix unmounts when
 * closed — so a workspace whose Integrations are shut costs nothing, and the
 * probes start when you open them. That is what the accordion tree did before
 * the menu replaced it.
 */
export function IntegrationsEnabledProvider({
  workspaceId,
  children,
}: {
  workspaceId: string;
  children: ReactNode;
}) {
  const enabled = useEnabledIntegrations(workspaceId);
  return (
    <EnabledIntegrationsContext.Provider value={enabled}>
      <BadgeWorkspaceContext.Provider value={workspaceId}>
        {children}
      </BadgeWorkspaceContext.Provider>
    </EnabledIntegrationsContext.Provider>
  );
}

/**
 * The badge for one integration row, or nothing. Renders null outside a
 * provider, which is what a row gets while its branch is still mounting.
 *
 * Built-in integrations badge from the provider's probe result. Plugin
 * integrations (slugs the built-in set does not know) badge from the plugin
 * registry's per-workspace enabled map — written by the owning plugin via
 * `host.setIntegrationEnabled` — so the badge is workspace-scoped and
 * reactive through the registry's existing subscription.
 */
export function IntegrationEnabledBadgeFor({ slug }: { slug: IntegrationSlug | string }) {
  const builtinEnabled = useContext(EnabledIntegrationsContext);
  if (builtinEnabled.has(slug as IntegrationSlug)) {
    return <IntegrationEnabledBadge />;
  }
  return <PluginIntegrationEnabledBadge slug={slug} />;
}

function PluginIntegrationEnabledBadge({ slug }: { slug: string }) {
  const registry = usePluginRegistry();
  const workspaceId = useContext(BadgeWorkspaceContext);
  if (!workspaceId) return null;
  if (!registry.isIntegrationEnabled(slug, workspaceId)) return null;
  return <IntegrationEnabledBadge />;
}
