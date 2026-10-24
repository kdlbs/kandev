import { useCallback, useEffect, useRef } from "react";
import { useOptionalAppStore, useOptionalAppStoreApi } from "@/components/state-provider";
import { useWebSocketClient } from "@/lib/ws/connection";
import { themeReference } from "@/lib/theme/theme-resolver";
import type { ThemeDefinition, ThemeSelection } from "@/lib/theme/theme-types";
import { invalidateThemeRequests, loadThemeContent, refreshThemeInventory } from "./theme-loader";

const emptyDefinitions: Record<string, ThemeDefinition> = {};
export function useThemeInventory(selection: ThemeSelection) {
  const store = useOptionalAppStoreApi();
  const client = useWebSocketClient();
  const inventory = useOptionalAppStore((s) => s.themeInventory, null);
  const definitions = useOptionalAppStore((s) => s.themeDefinitions, emptyDefinitions);
  const error = useOptionalAppStore((s) => s.themeError, null);
  const owner = useOptionalAppStore(
    (s) => `${s.auth.mode}:${s.auth.authenticated}:${s.auth.user?.id ?? ""}`,
    "",
  );
  const authenticated = useOptionalAppStore((s) => s.auth.authenticated, false);
  const previousOwner = useRef(owner);
  const ownerChanged = previousOwner.current !== owner;
  const refresh = useCallback(() => {
    if (store) void refreshThemeInventory(store);
  }, [store]);
  useEffect(() => {
    if (!store) return;
    if (previousOwner.current !== owner) store.getState().resetThemes();
    previousOwner.current = owner;
    invalidateThemeRequests(store);
    if (!authenticated) return;
    refresh();
    window.addEventListener("focus", refresh);
    const unsubscribe = client?.on("plugin.themes_changed", refresh);
    const disconnect = client?.onConnectionStatus((status) => {
      if (status === "connected") refresh();
    });
    return () => {
      window.removeEventListener("focus", refresh);
      unsubscribe?.();
      disconnect?.();
      invalidateThemeRequests(store);
    };
  }, [store, client, owner, authenticated, refresh]);
  useEffect(() => {
    if (!store || !authenticated) return;
    const descriptor = inventory?.themes.find(
      (d) => themeReference(d.package_id, d.key) === selection,
    );
    if (descriptor) void loadThemeContent(store, descriptor);
  }, [store, inventory, selection, authenticated, owner]);
  return {
    inventory: authenticated && !ownerChanged ? inventory : null,
    definitions: authenticated && !ownerChanged ? definitions : emptyDefinitions,
    error,
    refresh,
  };
}
