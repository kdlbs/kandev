"use client";

import { Component, type ErrorInfo, type ReactNode } from "react";
import { usePluginRegistry } from "@/lib/plugins/registry";
import type { SlotComponent } from "@/lib/plugins/types";

type PluginSlotErrorBoundaryProps = {
  slotName: string;
  children: ReactNode;
};

type PluginSlotErrorBoundaryState = {
  error: Error | null;
};

/**
 * Isolates a single plugin slot component. Plugin code runs in-process (see
 * PLUGIN-API.md security posture) — a throw anywhere in one plugin's slot
 * render must not tear down the host surface or other plugins' components.
 */
class PluginSlotErrorBoundary extends Component<
  PluginSlotErrorBoundaryProps,
  PluginSlotErrorBoundaryState
> {
  state: PluginSlotErrorBoundaryState = { error: null };

  static getDerivedStateFromError(error: Error): PluginSlotErrorBoundaryState {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    console.error(
      `[plugins] slot "${this.props.slotName}" component threw`,
      error,
      info.componentStack,
    );
  }

  render(): ReactNode {
    if (this.state.error) return null;
    return this.props.children;
  }
}

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
        <PluginSlotErrorBoundary key={`${name}-${index}`} slotName={name}>
          <SlotComponentImpl slotProps={slotProps} />
        </PluginSlotErrorBoundary>
      ))}
    </>
  );
}
