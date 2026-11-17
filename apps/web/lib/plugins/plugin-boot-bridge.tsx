"use client";

import { useEffect } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { bootPlugins } from "./boot";
import type { ActivePlugin } from "./types";
import type { SidebarGroupingProvider } from "@kdlbs/plugin-sdk";
import { sidebarTaskPageScope } from "@/lib/sidebar/sidebar-task-page-cache";
import {
  reconcileSidebarGroupingCatalogScope,
  refreshSidebarGroupingCatalog,
} from "./sidebar-grouping-catalog";
import { pluginRegistry } from "./registry";

type PluginBootBridgeProps = {
  plugins?: ActivePlugin[];
  sidebarGroupings?: readonly SidebarGroupingProvider[];
};

/**
 * Mounted once at the app root (inside `StateProvider`, see `src/main.tsx`).
 * Triggers `bootPlugins` as soon as the store + boot payload plugins are
 * available. Renders nothing — `bootPlugins` itself guards against
 * double-loading a store that has already booted, so repeated effect runs
 * (StrictMode double-invoke, re-renders) are safe no-ops.
 */
export function PluginBootBridge({ plugins, sidebarGroupings }: PluginBootBridgeProps) {
  const store = useAppStoreApi();
  const connectionStatus = useAppStore((state) => state.connection.status);
  const scope = useAppStore(sidebarTaskPageScope);

  useEffect(() => {
    bootPlugins({ plugins, sidebarGroupings }, store);
  }, [plugins, sidebarGroupings, store]);

  useEffect(() => {
    reconcileSidebarGroupingCatalogScope(store);
    if (connectionStatus !== "connected") return;
    pluginRegistry.invalidateSidebarGroupings();
    void refreshSidebarGroupingCatalog(store);
  }, [connectionStatus, scope, store]);

  return null;
}
