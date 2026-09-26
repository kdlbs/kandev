"use client";

import { createContext, useCallback, useContext, useEffect, useRef, type ReactNode } from "react";
import { LOCATION_CHANGE_EVENT } from "@/lib/routing/navigation-event";
import {
  SETTINGS_TARGET_REQUEST_EVENT,
  createSettingsTargetRegistry,
  revealSettingsTarget,
  settingsTargetFromHash,
  type SettingsTargetRegistry,
  type SettingsTargetRequestDetail,
} from "@/lib/settings-discovery/target";

const SettingsTargetContext = createContext<SettingsTargetRegistry | null>(null);

export function SettingsTargetProvider({
  children,
  revealTarget = revealSettingsTarget,
}: {
  children: ReactNode;
  revealTarget?: (element: HTMLElement) => void;
}) {
  const registryRef = useRef<SettingsTargetRegistry | null>(null);
  if (!registryRef.current) registryRef.current = createSettingsTargetRegistry(revealTarget);
  const registry = registryRef.current;

  useEffect(() => {
    const requestHashTarget = () => {
      const targetId = settingsTargetFromHash(window.location.hash);
      if (targetId) registry.request(targetId);
    };
    const requestExplicitTarget = (event: Event) => {
      const targetId = (event as CustomEvent<SettingsTargetRequestDetail>).detail?.targetId;
      if (targetId) registry.request(targetId);
    };

    window.addEventListener(LOCATION_CHANGE_EVENT, requestHashTarget);
    window.addEventListener("hashchange", requestHashTarget);
    window.addEventListener("popstate", requestHashTarget);
    window.addEventListener(SETTINGS_TARGET_REQUEST_EVENT, requestExplicitTarget);
    requestHashTarget();

    return () => {
      window.removeEventListener(LOCATION_CHANGE_EVENT, requestHashTarget);
      window.removeEventListener("hashchange", requestHashTarget);
      window.removeEventListener("popstate", requestHashTarget);
      window.removeEventListener(SETTINGS_TARGET_REQUEST_EVENT, requestExplicitTarget);
    };
  }, [registry]);

  return (
    <SettingsTargetContext.Provider value={registry}>{children}</SettingsTargetContext.Provider>
  );
}

export function useSettingsTargetRegistration(targetId?: string) {
  const registry = useContext(SettingsTargetContext);
  const unregisterRef = useRef<(() => void) | null>(null);

  return useCallback(
    (element: HTMLElement | null) => {
      unregisterRef.current?.();
      unregisterRef.current =
        element && targetId && registry ? registry.register(targetId, element) : null;
    },
    [registry, targetId],
  );
}
