"use client";

import { usePluginRegistry } from "@/lib/plugins/registry";
import { PluginErrorBoundary } from "./plugin-error-boundary";

export type PluginSlotProps = {
  /** Named slot to render — see PLUGIN-API.md for the initial set of slot names. */
  name: string;
  /** Forwarded to each registered component as `slotProps`. */
  slotProps?: unknown;
  /**
   * When set, render only the components registered by this plugin. Used by
   * owner-scoped slots (e.g. "plugin-settings" on a plugin's own settings
   * page) so the host isolates by owner and plugin authors don't have to gate
   * on the current plugin id themselves.
   */
  ownerPluginId?: string;
};

/**
 * Renders every plugin component registered for the named slot
 * (`registry.registerComponent(name, Component)`), each isolated behind its
 * own error boundary so one broken plugin can't break the host surface. Pass
 * `ownerPluginId` to restrict rendering to that plugin's own components.
 */
export function PluginSlot({ name, slotProps, ownerPluginId }: PluginSlotProps) {
  const registry = usePluginRegistry();
  const registrations = registry
    .getSlotRegistrations(name)
    .filter((registration) => !ownerPluginId || registration.pluginId === ownerPluginId);

  if (registrations.length === 0) return null;

  return (
    <>
      {registrations.map(({ registrationId, pluginId, Component }) => (
        <PluginErrorBoundary
          key={registrationId}
          context={`plugin "${pluginId}" slot "${name}" component`}
        >
          <Component slotProps={slotProps} />
        </PluginErrorBoundary>
      ))}
    </>
  );
}
