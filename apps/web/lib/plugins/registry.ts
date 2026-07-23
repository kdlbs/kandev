/**
 * Reactive singleton `PluginRegistry` (docs/plans/plugins/PLUGIN-API.md).
 *
 * Holds every registration made by every loaded plugin, tracks the owning
 * pluginId so a disabled/uninstalled plugin can be bulk-revoked, and exposes
 * a tiny external-store subscription so React components re-render on
 * registration changes (`usePluginRegistry()`).
 *
 * `pluginRegistry.forPlugin(pluginId)` returns the exact `PluginRegistry`
 * shape from the frozen contract (no pluginId param on the register*
 * methods) — this is what `host.ts` passes into a plugin's `initialize()`.
 */
import { useSyncExternalStore } from "react";
import type {
  NavItem,
  PluginRegistry,
  PluginRouteOptions,
  SlotComponent,
  WsHandler,
} from "./types";
import type { ComponentType } from "react";

interface Owned<T> {
  pluginId: string;
  value: T;
}

export interface RouteRegistration {
  path: string;
  Component: ComponentType;
  options?: PluginRouteOptions;
}

/** Route registration plus the owning pluginId — what `getRoutes()` returns. */
export interface PluginRouteRegistration extends RouteRegistration {
  pluginId: string;
}

interface SlotRegistration {
  registrationId: string;
  orderingId: string;
  slot: string;
  Component: SlotComponent;
}

/** Slot component plus its stable registry identity and owning plugin. */
export interface PluginSlotRegistration {
  registrationId: string;
  orderingId: string;
  pluginId: string;
  Component: SlotComponent;
}

interface WsHandlerRegistration {
  action: string;
  handler: WsHandler;
}

function removeByPlugin<T>(list: Owned<T>[], pluginId: string): Owned<T>[] {
  return list.filter((entry) => entry.pluginId !== pluginId);
}

class PluginRegistryStore {
  private routes: Owned<RouteRegistration>[] = [];
  private settingsRoutes: Owned<RouteRegistration>[] = [];
  private navItems: Owned<NavItem>[] = [];
  private slotComponents: Owned<SlotRegistration>[] = [];
  private wsHandlers: Owned<WsHandlerRegistration>[] = [];
  private nextSlotRegistrationId = 0;
  /** Display names from the boot payload, used for derived page-chrome titles. */
  private pluginNames = new Map<string, string>();
  private listeners = new Set<() => void>();
  private version = 0;

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };

  getVersion = (): number => this.version;

  registerRoute(
    pluginId: string,
    path: string,
    Component: ComponentType,
    options?: PluginRouteOptions,
  ): void {
    this.routes.push({ pluginId, value: { path, Component, options } });
    this.notify();
  }

  registerSettingsRoute(pluginId: string, path: string, Component: ComponentType): void {
    this.settingsRoutes.push({ pluginId, value: { path, Component } });
    this.notify();
  }

  registerNavItem(pluginId: string, item: NavItem): void {
    this.navItems.push({ pluginId, value: item });
    this.notify();
  }

  registerComponent(pluginId: string, slot: string, Component: SlotComponent): void {
    const ordinal = this.slotComponents.filter(
      (entry) => entry.pluginId === pluginId && entry.value.slot === slot,
    ).length;
    this.slotComponents.push({
      pluginId,
      value: {
        registrationId: `slot-registration-${this.nextSlotRegistrationId++}`,
        orderingId: pluginSlotOrderingId(pluginId, slot, ordinal),
        slot,
        Component,
      },
    });
    this.notify();
  }

  registerWsHandler(pluginId: string, action: string, handler: WsHandler): void {
    this.wsHandlers.push({ pluginId, value: { action, handler } });
    this.notify();
  }

  /** Bulk-revoke every registration owned by `pluginId` (disable/uninstall). */
  unregisterPlugin(pluginId: string): void {
    const before = this.totalCount();
    this.routes = removeByPlugin(this.routes, pluginId);
    this.settingsRoutes = removeByPlugin(this.settingsRoutes, pluginId);
    this.navItems = removeByPlugin(this.navItems, pluginId);
    this.slotComponents = removeByPlugin(this.slotComponents, pluginId);
    this.wsHandlers = removeByPlugin(this.wsHandlers, pluginId);
    this.pluginNames.delete(pluginId);
    if (this.totalCount() !== before) this.notify();
  }

  getRoutes(): PluginRouteRegistration[] {
    return this.routes.map((entry) => ({ ...entry.value, pluginId: entry.pluginId }));
  }

  /** Display name recorded by `forPlugin` (boot payload `ActivePlugin.name`). */
  getPluginName(pluginId: string): string | undefined {
    return this.pluginNames.get(pluginId);
  }

  getSettingsRoutes(): RouteRegistration[] {
    return this.settingsRoutes.map((entry) => entry.value);
  }

  getNavItems(): NavItem[] {
    return this.navItems.map((entry) => entry.value);
  }

  getSlotComponents(slot: string): SlotComponent[] {
    return this.getSlotRegistrations(slot).map((registration) => registration.Component);
  }

  /** Stable, plugin-owned slot registrations for host render boundaries. */
  getSlotRegistrations(slot: string): PluginSlotRegistration[] {
    return this.slotComponents
      .filter((entry) => entry.value.slot === slot)
      .map((entry) => ({
        registrationId: entry.value.registrationId,
        orderingId: entry.value.orderingId,
        pluginId: entry.pluginId,
        Component: entry.value.Component,
      }));
  }

  /**
   * Slot components for `slot` registered by `pluginId` only. Used by
   * owner-scoped slots (e.g. "plugin-settings") that render on a specific
   * plugin's own surface, so the host filters by owner instead of making
   * every plugin author gate on the current plugin id.
   */
  getSlotComponentsForPlugin(slot: string, pluginId: string): SlotComponent[] {
    return this.slotComponents
      .filter((entry) => entry.value.slot === slot && entry.pluginId === pluginId)
      .map((entry) => entry.value.Component);
  }

  getWsHandlers(action: string): WsHandler[] {
    return this.wsHandlers
      .filter((entry) => entry.value.action === action)
      .map((entry) => entry.value.handler);
  }

  /** Registry view scoped to one plugin — matches the frozen `PluginRegistry` contract. */
  forPlugin(pluginId: string, pluginName?: string): PluginRegistry {
    if (pluginName) this.pluginNames.set(pluginId, pluginName);
    return {
      registerRoute: (path, Component, options) =>
        this.registerRoute(pluginId, path, Component, options),
      registerNavItem: (item) => this.registerNavItem(pluginId, item),
      registerSettingsRoute: (path, Component) =>
        this.registerSettingsRoute(pluginId, path, Component),
      registerComponent: (slot, Component) => this.registerComponent(pluginId, slot, Component),
      registerWsHandler: (action, handler) => this.registerWsHandler(pluginId, action, handler),
    };
  }

  private totalCount(): number {
    return (
      this.routes.length +
      this.settingsRoutes.length +
      this.navItems.length +
      this.slotComponents.length +
      this.wsHandlers.length
    );
  }

  private notify(): void {
    this.version += 1;
    this.listeners.forEach((listener) => listener());
  }
}

function pluginSlotOrderingId(pluginId: string, slot: string, ordinal: number): string {
  return `plugin:${encodeURIComponent(pluginId)}:${encodeURIComponent(slot)}:${ordinal}`;
}

export const pluginRegistry = new PluginRegistryStore();

/** Snapshot hook: re-renders the caller whenever any plugin registration changes. */
export function usePluginRegistry(): PluginRegistryStore {
  useSyncExternalStore(
    pluginRegistry.subscribe,
    pluginRegistry.getVersion,
    pluginRegistry.getVersion,
  );
  return pluginRegistry;
}
