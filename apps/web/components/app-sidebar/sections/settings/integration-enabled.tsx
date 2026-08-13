"use client";

import { createContext, useContext, type ReactNode } from "react";

import { IntegrationEnabledBadge } from "@/components/settings/record-badges";
import {
  useEnabledIntegrations,
  type IntegrationSlug,
} from "@/hooks/domains/integrations/use-enabled-integrations";

const NONE: ReadonlySet<string> = new Set();

const EnabledIntegrationsContext = createContext<ReadonlySet<string>>(NONE);

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
      {children}
    </EnabledIntegrationsContext.Provider>
  );
}

/**
 * The badge for one integration row, or nothing. Renders null outside a
 * provider, which is what a row gets while its branch is still mounting.
 */
export function IntegrationEnabledBadgeFor({ slug }: { slug: IntegrationSlug }) {
  const enabled = useContext(EnabledIntegrationsContext);
  return enabled.has(slug) ? <IntegrationEnabledBadge /> : null;
}
