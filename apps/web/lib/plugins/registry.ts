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
import { i18n } from "@/lib/i18n";
import type {
  NavItem,
  IntegrationSettingsRegistration,
  PluginRegistry,
  PluginRouteOptions,
  PluginTranslationCatalogs,
  RepositoryProviderRegistration,
  ReviewProviderRegistration,
  SlotComponent,
  TaskActionRegistration,
  TaskFilterRegistration,
  TaskListFacetRegistration,
  TaskMenuActionRegistration,
  TaskPanelRegistration,
  WsHandler,
} from "./types";
import type { ComponentType } from "react";
import { pluginSlotOrderingId, taskActionKey } from "./registry-normalization";
import {
  wrapRepositoryProviderLifecycle,
  wrapReviewProviderLifecycle,
} from "./registry-provider-lifecycle";
import { PluginWorkLifecycle } from "./registry-work-lifecycle";
import { PluginProviderOwnership } from "./registry-provider-ownership";
import { registerPluginTranslations, unregisterPluginTranslations } from "./plugin-translations";
import type {
  PluginIntegrationSettingsRegistration,
  PluginKeybindingHandler,
  PluginLifecycleSnapshot,
  PluginLifecycleStatus,
  PluginNavRegistration,
  PluginRepositoryProviderRegistration,
  PluginReviewProviderRegistration,
  PluginRouteRegistration,
  PluginSlotRegistration,
  PluginTaskActionRegistration,
  PluginTaskFilterRegistration,
  PluginTaskMenuActionRegistration,
  PluginTaskListFacetRegistration,
  PluginTaskPanelRegistration,
  RouteRegistration,
} from "./registry-registration-types";
export * from "./registry-registration-types";

interface Owned<T> {
  pluginId: string;
  value: T;
}

interface SlotRegistration {
  registrationId: string;
  orderingId: string;
  slot: string;
  Component: SlotComponent;
}

interface WsHandlerRegistration {
  action: string;
  handler: WsHandler;
}

const CORE_INTEGRATION_SETTINGS_IDS = new Set([
  "azure-devops",
  "github",
  "gitlab",
  "jira",
  "linear",
  "sentry",
  "slack",
]);
const INTEGRATION_SETTINGS_ID_PATTERN = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
function removeByPlugin<T>(list: Owned<T>[], pluginId: string): Owned<T>[] {
  return list.filter((entry) => entry.pluginId !== pluginId);
}

class PluginRegistryStore {
  private routes: Owned<RouteRegistration>[] = [];
  private settingsRoutes: Owned<RouteRegistration>[] = [];
  private integrationSettings = new Map<string, Owned<IntegrationSettingsRegistration>>();
  /**
   * Plugin integration enabled state: integrationId -> (workspaceId ->
   * enabled). The registration owner is checked before a write, so two
   * integrations from one plugin cannot share a badge state and one plugin
   * cannot publish another plugin's state. Kept here — not in localStorage —
   * so it is workspace-scoped, survives via the plugin's own persisted
   * storage, and is reactive through the registry's existing notify/subscribe
   * cycle.
   */
  private integrationEnabled = new Map<string, Map<string, boolean>>();
  private navItems: Owned<NavItem>[] = [];
  private slotComponents: Owned<SlotRegistration>[] = [];
  private wsHandlers: Owned<WsHandlerRegistration>[] = [];
  private keybindingHandlers: Owned<PluginKeybindingHandler>[] = [];
  private repositoryProviders = new Map<string, Owned<RepositoryProviderRegistration>>();
  private taskActions = new Map<string, Owned<TaskActionRegistration>>();
  private reviewProviders = new Map<string, Owned<ReviewProviderRegistration>>();
  private providerOwnership = new PluginProviderOwnership();
  private workLifecycle = new PluginWorkLifecycle();
  private taskPanels: Owned<TaskPanelRegistration>[] = [];
  private taskMenuActions: Owned<TaskMenuActionRegistration>[] = [];
  private taskFilters: Owned<TaskFilterRegistration>[] = [];
  private taskListFacets: Owned<TaskListFacetRegistration>[] = [];
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

  constructor() {
    i18n.on("languageChanged", () => this.notify());
  }

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

  /**
   * Sets one plugin's integration enabled state for one workspace. No-op
   * (no notify) when the value is unchanged: plugins boot-sync every
   * workspace on initialize, and re-notifying identical values would
   * re-render every registry consumer — the sidebar badge flicker.
   */
  setIntegrationEnabled(
    pluginId: string,
    integrationId: string,
    workspaceId: string,
    enabled: boolean,
  ): void {
    if (this.integrationSettings.get(integrationId)?.pluginId !== pluginId) return;

    let byWorkspace = this.integrationEnabled.get(integrationId);
    if (!byWorkspace) {
      byWorkspace = new Map();
      this.integrationEnabled.set(integrationId, byWorkspace);
    }
    if (byWorkspace.get(workspaceId) === enabled) return;
    byWorkspace.set(workspaceId, enabled);
    this.notify();
  }

  getIntegrationEnabled(integrationId: string, workspaceId: string): boolean | undefined {
    return this.integrationEnabled.get(integrationId)?.get(workspaceId);
  }

  isIntegrationEnabled(integrationId: string, workspaceId: string): boolean {
    return this.getIntegrationEnabled(integrationId, workspaceId) === true;
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

  registerIntegrationSettings(
    pluginId: string,
    integration: IntegrationSettingsRegistration,
  ): void {
    if (!INTEGRATION_SETTINGS_ID_PATTERN.test(integration.id)) {
      throw new Error(
        `[plugins] integration settings id "${integration.id}" must be a URL-safe slug`,
      );
    }
    if (CORE_INTEGRATION_SETTINGS_IDS.has(integration.id)) {
      throw new Error(
        `[plugins] integration settings id "${integration.id}" is reserved by the host`,
      );
    }
    const existing = this.integrationSettings.get(integration.id);
    if (existing) {
      throw new Error(
        `[plugins] integration settings "${integration.id}" is already owned by "${existing.pluginId}"`,
      );
    }
    this.integrationSettings.set(integration.id, { pluginId, value: integration });
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

  registerTaskFilter(pluginId: string, registration: TaskFilterRegistration): void {
    this.taskFilters.push({ pluginId, value: registration });
    this.notify();
  }

  registerTaskListFacet(pluginId: string, registration: TaskListFacetRegistration): void {
    if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(registration.id)) {
      throw new Error(`[plugins] task-list facet id "${registration.id}" must be URL-safe`);
    }
    if (
      this.taskListFacets.some(
        (entry) => entry.pluginId === pluginId && entry.value.id === registration.id,
      )
    ) {
      throw new Error(
        `[plugins] task-list facet "${registration.id}" is already registered by "${pluginId}"`,
      );
    }
    this.taskListFacets.push({ pluginId, value: registration });
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

  /**
   * Supplies manifest-backed repository provider declarations for a plugin.
   * Kept separate from `forPlugin` so older boot payloads/plugins retain their
   * existing routes, slots, websocket handlers, and keybindings during the
   * additive contract rollout.
   */
  setDeclaredRepositoryProviderIds(pluginId: string, ids: string[]): void {
    this.providerOwnership.setDeclarations(pluginId, ids);
  }

  registerRepositoryProvider(pluginId: string, provider: RepositoryProviderRegistration): void {
    this.providerOwnership.claim(pluginId, provider.id);
    if (this.repositoryProviders.has(provider.id)) {
      throw new Error(`[plugins] repository provider "${provider.id}" is already registered`);
    }
    this.repositoryProviders.set(provider.id, {
      pluginId,
      value: this.withRepositoryProviderLifecycle(pluginId, provider),
    });
    this.notify();
  }

  registerTaskAction(pluginId: string, action: TaskActionRegistration): void {
    const key = taskActionKey(pluginId, action.id);
    if (this.taskActions.has(key)) {
      throw new Error(
        `[plugins] task action "${action.id}" is already registered by "${pluginId}"`,
      );
    }
    this.taskActions.set(key, { pluginId, value: action });
    this.notify();
  }

  registerReviewProvider(pluginId: string, provider: ReviewProviderRegistration): void {
    this.providerOwnership.claim(pluginId, provider.id);
    if (this.reviewProviders.has(provider.id)) {
      throw new Error(`[plugins] review provider "${provider.id}" is already registered`);
    }
    this.reviewProviders.set(provider.id, {
      pluginId,
      value: this.withReviewProviderLifecycle(pluginId, provider),
    });
    this.notify();
  }

  registerTranslations(pluginId: string, catalogs: PluginTranslationCatalogs): void {
    registerPluginTranslations(pluginId, catalogs);
    this.notify();
  }

  /** Bulk-revoke every registration owned by `pluginId` (disable/uninstall). */
  unregisterPlugin(pluginId: string): void {
    const before = this.totalCount();
    this.routes = removeByPlugin(this.routes, pluginId);
    this.settingsRoutes = removeByPlugin(this.settingsRoutes, pluginId);
    const removedIntegrationIds: string[] = [];
    this.integrationSettings.forEach((entry, id) => {
      if (entry.pluginId === pluginId) {
        this.integrationSettings.delete(id);
        removedIntegrationIds.push(id);
      }
    });
    this.navItems = removeByPlugin(this.navItems, pluginId);
    this.slotComponents = removeByPlugin(this.slotComponents, pluginId);
    this.wsHandlers = removeByPlugin(this.wsHandlers, pluginId);
    this.keybindingHandlers = removeByPlugin(this.keybindingHandlers, pluginId);
    this.repositoryProviders.forEach((entry, id) => {
      if (entry.pluginId === pluginId) this.repositoryProviders.delete(id);
    });
    this.taskActions.forEach((entry, id) => {
      if (entry.pluginId === pluginId) this.taskActions.delete(id);
    });
    this.reviewProviders.forEach((entry, id) => {
      if (entry.pluginId === pluginId) this.reviewProviders.delete(id);
    });
    const removedTranslations = unregisterPluginTranslations(pluginId);
    this.abortPluginWork(pluginId);
    this.providerOwnership.releasePlugin(pluginId);
    this.taskPanels = removeByPlugin(this.taskPanels, pluginId);
    this.taskMenuActions = removeByPlugin(this.taskMenuActions, pluginId);
    this.taskFilters = removeByPlugin(this.taskFilters, pluginId);
    this.taskListFacets = removeByPlugin(this.taskListFacets, pluginId);
    this.pluginNames.delete(pluginId);
    this.declaredKeybindingIds.delete(pluginId);
    const removedEnabledState = removedIntegrationIds.reduce(
      (removed, integrationId) => this.integrationEnabled.delete(integrationId) || removed,
      false,
    );
    if (removedEnabledState || removedTranslations || this.totalCount() !== before) this.notify();
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

  getIntegrationSettings(): PluginIntegrationSettingsRegistration[] {
    return Array.from(this.integrationSettings.values()).map((entry) => ({
      ...entry.value,
      pluginId: entry.pluginId,
    }));
  }

  getIntegrationSetting(id: string): PluginIntegrationSettingsRegistration | undefined {
    const entry = this.integrationSettings.get(id);
    return entry ? { ...entry.value, pluginId: entry.pluginId } : undefined;
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

  /** Every active repository provider in registration order. */
  getRepositoryProviders(): PluginRepositoryProviderRegistration[] {
    return Array.from(this.repositoryProviders.values()).map((entry) => ({
      ...entry.value,
      pluginId: entry.pluginId,
    }));
  }

  /** The active owner and lifecycle-wrapped provider for `providerId`, if any. */
  getRepositoryProvider(providerId: string): PluginRepositoryProviderRegistration | undefined {
    const entry = this.repositoryProviders.get(providerId);
    return entry ? { ...entry.value, pluginId: entry.pluginId } : undefined;
  }

  /** Active task actions, optionally filtered by their target menu placement. */
  getTaskActions(placement?: TaskActionRegistration["placement"]): PluginTaskActionRegistration[] {
    return Array.from(this.taskActions.values())
      .filter((entry) => !placement || entry.value.placement === placement)
      .map((entry) => ({ ...entry.value, pluginId: entry.pluginId }));
  }

  /** Active review providers in deterministic display order. */
  getReviewProviders(): PluginReviewProviderRegistration[] {
    return Array.from(this.reviewProviders.values())
      .map((entry) => ({ ...entry.value, pluginId: entry.pluginId }))
      .sort((a, b) => a.order - b.order || a.id.localeCompare(b.id));
  }

  /** The active owner and lifecycle-wrapped review provider for `providerId`, if any. */
  getReviewProvider(providerId: string): PluginReviewProviderRegistration | undefined {
    const entry = this.reviewProviders.get(providerId);
    return entry ? { ...entry.value, pluginId: entry.pluginId } : undefined;
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

  /** Every registered task filter, in registration order. */
  getTaskFilters(): PluginTaskFilterRegistration[] {
    return this.taskFilters.map((entry) => ({ ...entry.value, pluginId: entry.pluginId }));
  }

  getTaskListFacets(): PluginTaskListFacetRegistration[] {
    return this.taskListFacets.map((entry) => ({ ...entry.value, pluginId: entry.pluginId }));
  }

  /** Registry view scoped to one plugin — matches the frozen `PluginRegistry` contract. */
  forPlugin(pluginId: string, pluginName?: string): PluginRegistry {
    if (pluginName) this.pluginNames.set(pluginId, pluginName);
    return {
      registerTranslations: (catalogs) => this.registerTranslations(pluginId, catalogs),
      registerRoute: (path, Component, options) =>
        this.registerRoute(pluginId, path, Component, options),
      registerNavItem: (item) => this.registerNavItem(pluginId, item),
      registerSettingsRoute: (path, Component) =>
        this.registerSettingsRoute(pluginId, path, Component),
      registerIntegrationSettings: (integration) =>
        this.registerIntegrationSettings(pluginId, integration),
      registerComponent: (slot, Component) => this.registerComponent(pluginId, slot, Component),
      registerWsHandler: (action, handler) => this.registerWsHandler(pluginId, action, handler),
      registerKeybinding: (id, handler) => this.registerKeybinding(pluginId, id, handler),
      registerRepositoryProvider: (provider) => this.registerRepositoryProvider(pluginId, provider),
      registerTaskAction: (action) => this.registerTaskAction(pluginId, action),
      registerReviewProvider: (provider) => this.registerReviewProvider(pluginId, provider),
      registerTaskPanel: (registration) => this.registerTaskPanel(pluginId, registration),
      registerTaskMenuAction: (registration) => this.registerTaskMenuAction(pluginId, registration),
      registerTaskFilter: (registration) => this.registerTaskFilter(pluginId, registration),
      registerTaskListFacet: (registration) => this.registerTaskListFacet(pluginId, registration),
    };
  }

  private withRepositoryProviderLifecycle(
    pluginId: string,
    provider: RepositoryProviderRegistration,
  ): RepositoryProviderRegistration {
    return wrapRepositoryProviderLifecycle(provider, (signal, operation) =>
      this.workLifecycle.run(pluginId, signal, operation),
    );
  }

  private withReviewProviderLifecycle(
    pluginId: string,
    provider: ReviewProviderRegistration,
  ): ReviewProviderRegistration {
    return wrapReviewProviderLifecycle(
      provider,
      (signal, operation) => this.workLifecycle.run(pluginId, signal, operation),
      (unsubscribe) => this.workLifecycle.trackSubscription(pluginId, unsubscribe),
    );
  }

  private abortPluginWork(pluginId: string): void {
    this.workLifecycle.abort(pluginId);
  }

  private totalCount(): number {
    return (
      this.routes.length +
      this.settingsRoutes.length +
      this.integrationSettings.size +
      this.navItems.length +
      this.slotComponents.length +
      this.wsHandlers.length +
      this.keybindingHandlers.length +
      this.repositoryProviders.size +
      this.taskActions.size +
      this.reviewProviders.size +
      this.taskPanels.length +
      this.taskMenuActions.length +
      this.taskFilters.length +
      this.taskListFacets.length
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
