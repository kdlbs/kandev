"use client";

import type { ComponentType, ReactNode } from "react";
import { PageTopbar } from "@/components/page-topbar";
import { lookupPluginIcon } from "@/lib/plugins/icons";
import { pluginRegistry } from "@/lib/plugins/registry";
import type { PluginRouteRegistration } from "@/lib/plugins/registry";
import type { NavItem, PluginPageChrome } from "@/lib/plugins/types";

export interface ResolvedPageChrome {
  title: string;
  subtitle?: string;
  icon?: string;
  backHref?: string;
  backLabel?: string;
  Actions?: ComponentType;
}

/**
 * Resolves the topbar chrome for a plugin route registration
 * (`registerRoute(path, Component, { topbar })`). Returns null when the
 * plugin opted out (`topbar: false`). Title fallback order: explicit chrome
 * title → the plugin's nav-item label registered for this path → the
 * plugin's display name → the route path. The icon similarly falls back to
 * the matching nav item's icon.
 */
export function resolvePluginPageChrome(
  registration: PluginRouteRegistration,
  navItems: NavItem[],
  pluginName: string | undefined,
): ResolvedPageChrome | null {
  const topbar = registration.options?.topbar ?? true;
  if (topbar === false) return null;
  const chrome: PluginPageChrome = topbar === true ? {} : topbar;
  const navItem = navItems.find((item) => item.path === registration.path);
  return {
    title: chrome.title ?? navItem?.label ?? pluginName ?? registration.path,
    subtitle: chrome.subtitle,
    icon: chrome.icon ?? navItem?.icon,
    backHref: chrome.backHref,
    backLabel: chrome.backLabel,
    Actions: chrome.actions,
  };
}

type PluginPageFrameProps = {
  registration: PluginRouteRegistration;
  children: ReactNode;
};

/**
 * Wraps a plugin-registered route in the same page shell first-party pages
 * use: a `PageTopbar` title bar above a scrollable content area. Renders the
 * children bare (full-bleed) when the registration opted out via
 * `topbar: false`.
 */
export function PluginPageFrame({ registration, children }: PluginPageFrameProps) {
  const chrome = resolvePluginPageChrome(
    registration,
    pluginRegistry.getNavItems(),
    pluginRegistry.getPluginName(registration.pluginId),
  );
  if (!chrome) return <>{children}</>;

  const Icon = lookupPluginIcon(chrome.icon);
  const Actions = chrome.Actions;
  return (
    <div className="flex h-screen w-full flex-col bg-background">
      <PageTopbar
        title={chrome.title}
        subtitle={chrome.subtitle}
        icon={Icon ? <Icon className="h-4 w-4" /> : undefined}
        backHref={chrome.backHref}
        backLabel={chrome.backLabel}
        actions={Actions ? <Actions /> : undefined}
      />
      <div className="min-h-0 flex-1 overflow-auto">{children}</div>
    </div>
  );
}
