"use client";

import { useEffect } from "react";
import { useAppStoreApi } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { bootPlugins } from "./boot";
import type { ActivePlugin } from "./types";

type PluginBootBridgeProps = {
  plugins?: ActivePlugin[];
};

/**
 * Mounted once at the app root (inside `StateProvider`, see `src/main.tsx`).
 * Triggers `bootPlugins` as soon as the store + boot payload plugins are
 * available. Renders nothing — `bootPlugins` itself guards against
 * double-loading a store that has already booted, so repeated effect runs
 * (StrictMode double-invoke, re-renders) are safe no-ops.
 */
export function PluginBootBridge({ plugins }: PluginBootBridgeProps) {
  const store = useAppStoreApi();
  const pluginsEnabled = useFeature("plugins");

  useEffect(() => {
    bootPlugins({ plugins }, store, pluginsEnabled);
  }, [plugins, store, pluginsEnabled]);

  return null;
}
