"use client";

import { useCallback, useEffect, useMemo } from "react";
import { usePathname, useRouter, useSearchParams } from "@/lib/routing/client-router";
import { runWithNavigationBlockerBypassed } from "@/lib/routing/navigation-guard";
import {
  SETTINGS_TARGET_REQUEST_EVENT,
  settingsTargetFromHash,
  type SettingsTargetRequestDetail,
} from "@/lib/settings-discovery/target";

export type SettingsTabId = string;

export type UseSettingsTabOptions = {
  tabs: readonly SettingsTabId[];
  defaultTab: SettingsTabId;
  targetToTab?: Readonly<Record<string, SettingsTabId>>;
};

export function resolveSettingsTab(
  value: string | null,
  tabs: readonly SettingsTabId[],
  defaultTab: SettingsTabId,
): SettingsTabId {
  return value && tabs.includes(value) ? value : defaultTab;
}

function currentTargetTab(targetToTab: Readonly<Record<string, SettingsTabId>>): string | null {
  if (typeof window === "undefined") return null;
  const targetId = settingsTargetFromHash(window.location.hash);
  return targetId ? (targetToTab[targetId] ?? null) : null;
}

function tabHref(pathname: string, tab: string, removeHash: boolean): string {
  const params = new URLSearchParams(typeof window === "undefined" ? "" : window.location.search);
  params.set("tab", tab);
  const query = params.toString();
  const hash = removeHash || typeof window === "undefined" ? "" : window.location.hash;
  return `${pathname}${query ? `?${query}` : ""}${hash}`;
}

export function useSettingsTab({ tabs, defaultTab, targetToTab = {} }: UseSettingsTabOptions) {
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const router = useRouter();
  const value = useMemo(
    () =>
      currentTargetTab(targetToTab) ??
      resolveSettingsTab(searchParams.get("tab"), tabs, defaultTab),
    [defaultTab, searchParams, tabs, targetToTab],
  );

  const selectTab = useCallback(
    (nextTab: string, removeHash = true) => {
      if (!tabs.includes(nextTab) || pathname !== window.location.pathname) return;
      const href = tabHref(pathname, nextTab, removeHash);
      runWithNavigationBlockerBypassed(() => router.replace(href, { scroll: false }));
    },
    [pathname, router, tabs],
  );

  useEffect(() => {
    const targetTab = currentTargetTab(targetToTab);
    if (targetTab && targetTab !== value) selectTab(targetTab, false);
  }, [selectTab, targetToTab, value]);

  useEffect(() => {
    const requestTargetTab = (event: Event) => {
      const targetId = (event as CustomEvent<SettingsTargetRequestDetail>).detail?.targetId;
      const targetTab = targetId ? targetToTab[targetId] : undefined;
      if (targetTab && targetTab !== value) selectTab(targetTab, false);
    };
    window.addEventListener(SETTINGS_TARGET_REQUEST_EVENT, requestTargetTab);
    return () => window.removeEventListener(SETTINGS_TARGET_REQUEST_EVENT, requestTargetTab);
  }, [selectTab, targetToTab, value]);

  return { value, selectTab };
}
