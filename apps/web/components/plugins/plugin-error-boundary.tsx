"use client";

import { Component, type ErrorInfo, type ReactNode } from "react";

type PluginErrorBoundaryProps = {
  /** Identifies what threw in the console log, e.g. `slot "task-sidebar"` or `route "/plugins/hello"`. */
  context: string;
  /** Rendered in place of `children` after a throw. Defaults to nothing (silent). */
  fallback?: ReactNode;
  children: ReactNode;
};

type PluginErrorBoundaryState = {
  error: Error | null;
};

/**
 * Isolates plugin-owned React output (a slot component, a top-level plugin
 * route, or a plugin settings route) so a throw anywhere in one plugin's
 * render can't tear down the host shell or other plugins — plugin code runs
 * in-process (see PLUGIN-API.md security posture).
 */
export class PluginErrorBoundary extends Component<
  PluginErrorBoundaryProps,
  PluginErrorBoundaryState
> {
  state: PluginErrorBoundaryState = { error: null };

  static getDerivedStateFromError(error: Error): PluginErrorBoundaryState {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    console.error(`[plugins] ${this.props.context} threw`, error, info.componentStack);
  }

  render(): ReactNode {
    if (this.state.error) return this.props.fallback ?? null;
    return this.props.children;
  }
}

/** Shared fallback for a plugin-owned top-level or settings route that threw during render. */
export function PluginRouteFallback() {
  return (
    <div className="flex flex-1 items-center justify-center p-8 text-sm text-muted-foreground">
      This plugin page failed to load.
    </div>
  );
}
