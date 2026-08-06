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
  TaskMenuActionRegistration,
  TaskPanelRegistration,
  WsHandler,
} from "./types";
import type { ComponentType } from "react";

interface Owned<T> {
  pluginId: string;
  value: T;
}

/** A handler bound via `PluginRegistry.registerKeybinding`. */
export interface PluginKeybindingHandler {
  /** Plugin-local keybinding id (matches `ui.keybindings[].id`). */
  id: string;
  handler: (event: KeyboardEvent) => void;
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

/**
 * Nav item plus the owning pluginId — what `getNavRegistrations()` returns.
 * Navigation needs the owner because `NavItem.id` is plugin-local: two plugins
 * may register the same id, and the navigation manifest builds its React keys
 * from it (`lib/navigation/plugin-destinations.ts`).
 */
export interface PluginNavRegistration extends NavItem {
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

/** Task panel registration plus the owning pluginId — what `getTaskPanels()` returns. */
export interface PluginTaskPanelRegistration extends TaskPanelRegistration {
  pluginId: string;
}

/** Task menu action registration plus the owning pluginId. */
export interface PluginTaskMenuActionRegistration extends TaskMenuActionRegistration {
  pluginId: string;
}

/** Host-owned lifecycle states used to reconcile registrations with UI state. */
export type PluginLifecycleStatus = "loading" | "ready" | "failed" | "removed";

/** A generation-fenced lifecycle snapshot; never exposed through PluginRegistry. */
export interface PluginLifecycleSnapshot {
  status: PluginLifecycleStatus;
  generation: number;
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
  private keybindingHandlers: Owned<PluginKeybindingHandler>[] = [];
  private taskPanels: Owned<TaskPanelRegistration>[] = [];
  private taskMenuActions: Owned<TaskMenuActionRegistration>[] = [];
  private nextSlotRegistrationId = 0;
  /** Display names from the boot payload, used for derived page-chrome titles. */
  private pluginNames = new Map<string, string>();
  /** Host-owned lifecycle state; plugin-facing registries only expose registrations. */
  private pluginLifecycles = new Map<string, PluginLifecycleSnapshot>();
  /**
   * Keybinding ids declared in each plugin's `ui.keybindings` manifest,
   * synced by the shortcut dispatcher (`hooks/use-plugin-shortcuts.ts`) from
   * the plugin records store. Used only to warn on `registerKeybinding`
   * calls for an id the manifest never declared — an empty/missing entry
   * (descriptors not loaded yet) skips the check rather than false-warning.
   */
  private declaredKeybindingIds = new Map<string, Set<string>>();
  private listeners = new Set<() => void>();
  private version = 0;

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };

  getVersion = (): number => this.version;

  getPluginLifecycle(pluginId: string): PluginLifecycleSnapshot | undefined {
    const snapshot = this.pluginLifecycles.get(pluginId);
    return snapshot ? { ...snapshot } : undefined;
  }

  markPluginLoading(pluginId: string, generation: number): void {
    this.setPluginLifecycle(pluginId, "loading", generation);
  }

  markPluginReady(pluginId: string, generation: number): void {
    this.setPluginLifecycle(pluginId, "ready", generation);
  }

  markPluginFailed(pluginId: string, generation: number): void {
    this.setPluginLifecycle(pluginId, "failed", generation);
  }

  markPluginRemoved(pluginId: string, generation: number): void {
    this.setPluginLifecycle(pluginId, "removed", generation);
  }

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

  /**
   * Removes exactly one previously registered WS handler (matched by
   * reference equality), without touching any of the plugin's other
   * registrations. Used by `host.storage.subscribe`'s returned unsubscribe
   * function so an individual subscription can end without the plugin being
   * disabled/uninstalled (which already bulk-revokes via unregisterPlugin).
   * A no-op if the handler is already gone (e.g. the plugin was unregistered
   * first).
   */
  unregisterWsHandler(pluginId: string, action: string, handler: WsHandler): void {
    const index = this.wsHandlers.findIndex(
      (entry) =>
        entry.pluginId === pluginId &&
        entry.value.action === action &&
        entry.value.handler === handler,
    );
    if (index === -1) return;
    this.wsHandlers.splice(index, 1);
    this.notify();
  }

  registerKeybinding(pluginId: string, id: string, handler: (event: KeyboardEvent) => void): void {
    const declared = this.declaredKeybindingIds.get(pluginId);
    if (declared && !declared.has(id)) {
      console.warn(
        `[plugins] "${pluginId}" registered a keybinding handler for id "${id}", which is not declared in its ui.keybindings manifest`,
      );
    }
    this.keybindingHandlers.push({ pluginId, value: { id, handler } });
    this.notify();
  }

  registerTaskPanel(pluginId: string, registration: TaskPanelRegistration): void {
    this.taskPanels.push({ pluginId, value: registration });
    this.notify();
  }

  registerTaskMenuAction(pluginId: string, registration: TaskMenuActionRegistration): void {
    this.taskMenuActions.push({ pluginId, value: registration });
    this.notify();
  }

  /**
   * Records the keybinding ids declared in `pluginId`'s `ui.keybindings`
   * manifest, so `registerKeybinding` can warn on an undeclared id. Safe to
   * call repeatedly (e.g. every time the plugin records store refreshes).
   */
  setDeclaredKeybindingIds(pluginId: string, ids: string[]): void {
    this.declaredKeybindingIds.set(pluginId, new Set(ids));
  }

  /** Bulk-revoke every registration owned by `pluginId` (disable/uninstall). */
  unregisterPlugin(pluginId: string): void {
    const before = this.totalCount();
    this.routes = removeByPlugin(this.routes, pluginId);
    this.settingsRoutes = removeByPlugin(this.settingsRoutes, pluginId);
    this.navItems = removeByPlugin(this.navItems, pluginId);
    this.slotComponents = removeByPlugin(this.slotComponents, pluginId);
    this.wsHandlers = removeByPlugin(this.wsHandlers, pluginId);
    this.keybindingHandlers = removeByPlugin(this.keybindingHandlers, pluginId);
    this.taskPanels = removeByPlugin(this.taskPanels, pluginId);
    this.taskMenuActions = removeByPlugin(this.taskMenuActions, pluginId);
    this.pluginNames.delete(pluginId);
    this.declaredKeybindingIds.delete(pluginId);
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

  /**
   * Nav items without their owner. Use `getNavRegistrations()` for anything that
   * needs a globally unique identity; this stays for callers that only read the
   * item's own fields (e.g. deriving a page title from a path).
   */
  getNavItems(): NavItem[] {
    return this.navItems.map((entry) => entry.value);
  }

  /** Nav items plus the pluginId that registered each one, in registration order. */
  getNavRegistrations(): PluginNavRegistration[] {
    return this.navItems.map((entry) => ({ ...entry.value, pluginId: entry.pluginId }));
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

  /**
   * All registered keybinding handlers plus their owning pluginId, in
   * registration order. Registration order is the dispatch-order tiebreaker
   * when two plugins bind the same effective combo (see
   * `hooks/use-plugin-shortcuts.ts`).
   */
  getKeybindingHandlers(): (PluginKeybindingHandler & { pluginId: string })[] {
    return this.keybindingHandlers.map((entry) => ({ ...entry.value, pluginId: entry.pluginId }));
  }

  /** The `pluginId`'s bound handler for `id`, if any (first match wins). */
  getKeybindingHandler(pluginId: string, id: string): ((event: KeyboardEvent) => void) | undefined {
    return this.keybindingHandlers.find(
      (entry) => entry.pluginId === pluginId && entry.value.id === id,
    )?.value.handler;
  }

  /** Every registered task panel, in registration order. */
  getTaskPanels(): PluginTaskPanelRegistration[] {
    return this.taskPanels.map((entry) => ({ ...entry.value, pluginId: entry.pluginId }));
  }

  /** The registration for a specific `pluginId`/panel `id`, if still registered. */
  getTaskPanel(pluginId: string, id: string): PluginTaskPanelRegistration | undefined {
    const entry = this.taskPanels.find(
      (candidate) => candidate.pluginId === pluginId && candidate.value.id === id,
    );
    return entry ? { ...entry.value, pluginId: entry.pluginId } : undefined;
  }

  /** Every registered task menu action, optionally filtered to one group. */
  getTaskMenuActions(
    group?: TaskMenuActionRegistration["group"],
  ): PluginTaskMenuActionRegistration[] {
    return this.taskMenuActions
      .filter((entry) => !group || entry.value.group === group)
      .map((entry) => ({ ...entry.value, pluginId: entry.pluginId }));
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
      registerKeybinding: (id, handler) => this.registerKeybinding(pluginId, id, handler),
      registerTaskPanel: (registration) => this.registerTaskPanel(pluginId, registration),
      registerTaskMenuAction: (registration) => this.registerTaskMenuAction(pluginId, registration),
    };
  }

  private totalCount(): number {
    return (
      this.routes.length +
      this.settingsRoutes.length +
      this.navItems.length +
      this.slotComponents.length +
      this.wsHandlers.length +
      this.keybindingHandlers.length +
      this.taskPanels.length +
      this.taskMenuActions.length
    );
  }

  private notify(): void {
    this.version += 1;
    this.listeners.forEach((listener) => listener());
  }

  private setPluginLifecycle(
    pluginId: string,
    status: PluginLifecycleStatus,
    generation: number,
  ): void {
    const current = this.pluginLifecycles.get(pluginId);
    if (
      current &&
      (generation < current.generation ||
        (generation === current.generation && current.status === status))
    ) {
      return;
    }
    this.pluginLifecycles.set(pluginId, { status, generation });
    this.notify();
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
