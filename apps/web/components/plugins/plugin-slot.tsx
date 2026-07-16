"use client";

import { usePluginRegistry } from "@/lib/plugins/registry";
import type { SlotComponent } from "@/lib/plugins/types";
import { PluginErrorBoundary } from "./plugin-error-boundary";

export type PluginSlotProps = {
  /** Named slot to render — see PLUGIN-API.md for the initial set of slot names. */
  name: string;
  /** Forwarded to each registered component as `slotProps`. */
  slotProps?: unknown;
};

/**
 * Renders every plugin component registered for the named slot
 * (`registry.registerComponent(name, Component)`), each isolated behind its
 * own error boundary so one broken plugin can't break the host surface.
 */
export function PluginSlot({ name, slotProps }: PluginSlotProps) {
  const registry = usePluginRegistry();
  const components = registry.getSlotComponents(name);

  if (components.length === 0) return null;

  return (
    <>
      {components.map((SlotComponentImpl: SlotComponent, index) => (
        <PluginErrorBoundary key={`${name}-${index}`} context={`slot "${name}" component`}>
          <SlotComponentImpl slotProps={slotProps} />
        </PluginErrorBoundary>
      ))}
    </>
  );
}
