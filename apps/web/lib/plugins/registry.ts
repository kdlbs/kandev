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
  IntegrationSettingsRegistration,
  PluginRegistry,
  PluginRouteOptions,
  RepositoryProviderRegistration,
  ReviewItemSummary,
  ReviewTaskPipelineState,
  ReviewTaskStatus,
  ReviewProviderRegistration,
  SlotComponent,
  TaskActionRegistration,
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

/** A repository provider paired with the plugin that currently owns it. */
export interface PluginRepositoryProviderRegistration extends RepositoryProviderRegistration {
  pluginId: string;
}

/** A task action paired with the plugin that registered it. */
export interface PluginTaskActionRegistration extends TaskActionRegistration {
  pluginId: string;
}

/** A review provider paired with the plugin that currently owns it. */
export interface PluginReviewProviderRegistration extends ReviewProviderRegistration {
  pluginId: string;
}

/** Integration settings contribution paired with its active plugin owner. */
export interface PluginIntegrationSettingsRegistration extends IntegrationSettingsRegistration {
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
const CORE_REPOSITORY_PROVIDER_IDS = new Set(["github", "gitlab", "azure_devops"]);

function removeByPlugin<T>(list: Owned<T>[], pluginId: string): Owned<T>[] {
  return list.filter((entry) => entry.pluginId !== pluginId);
}

class PluginRegistryStore {
  private routes: Owned<RouteRegistration>[] = [];
  private settingsRoutes: Owned<RouteRegistration>[] = [];
  private integrationSettings = new Map<string, Owned<IntegrationSettingsRegistration>>();
  private navItems: Owned<NavItem>[] = [];
  private slotComponents: Owned<SlotRegistration>[] = [];
  private wsHandlers: Owned<WsHandlerRegistration>[] = [];
  private keybindingHandlers: Owned<PluginKeybindingHandler>[] = [];
  private repositoryProviders = new Map<string, Owned<RepositoryProviderRegistration>>();
  private taskActions = new Map<string, Owned<TaskActionRegistration>>();
  private reviewProviders = new Map<string, Owned<ReviewProviderRegistration>>();
  /** One live plugin owns each provider ID across repository and review registrations. */
  private providerOwners = new Map<string, string>();
  /** Present only after the host has supplied manifest-declared provider IDs. */
  private declaredRepositoryProviderIds = new Map<string, Set<string>>();
  private abortControllersByPlugin = new Map<string, Set<AbortController>>();
  private reviewUnsubscribersByPlugin = new Map<string, Set<() => void>>();
  private nextSlotRegistrationId = 0;
  /** Display names from the boot payload, used for derived page-chrome titles. */
  private pluginNames = new Map<string, string>();
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
    this.declaredRepositoryProviderIds.set(pluginId, new Set(ids));
  }

  registerRepositoryProvider(pluginId: string, provider: RepositoryProviderRegistration): void {
    this.claimProvider(pluginId, provider.id);
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
    this.claimProvider(pluginId, provider.id);
    if (this.reviewProviders.has(provider.id)) {
      throw new Error(`[plugins] review provider "${provider.id}" is already registered`);
    }
    this.reviewProviders.set(provider.id, {
      pluginId,
      value: this.withReviewProviderLifecycle(pluginId, provider),
    });
    this.notify();
  }

  /** Bulk-revoke every registration owned by `pluginId` (disable/uninstall). */
  unregisterPlugin(pluginId: string): void {
    const before = this.totalCount();
    this.routes = removeByPlugin(this.routes, pluginId);
    this.settingsRoutes = removeByPlugin(this.settingsRoutes, pluginId);
    this.integrationSettings.forEach((entry, id) => {
      if (entry.pluginId === pluginId) this.integrationSettings.delete(id);
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
    this.abortPluginWork(pluginId);
    this.providerOwners.forEach((owner, providerId) => {
      if (owner === pluginId) this.providerOwners.delete(providerId);
    });
    this.pluginNames.delete(pluginId);
    this.declaredKeybindingIds.delete(pluginId);
    this.declaredRepositoryProviderIds.delete(pluginId);
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

  /** Registry view scoped to one plugin — matches the frozen `PluginRegistry` contract. */
  forPlugin(pluginId: string, pluginName?: string): PluginRegistry {
    if (pluginName) this.pluginNames.set(pluginId, pluginName);
    return {
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
    };
  }

  private claimProvider(pluginId: string, providerId: string): void {
    if (CORE_REPOSITORY_PROVIDER_IDS.has(providerId.trim().toLowerCase())) {
      throw new Error(`[plugins] provider "${providerId}" is reserved by the host`);
    }
    const declared = this.declaredRepositoryProviderIds.get(pluginId);
    if (declared && !declared.has(providerId)) {
      throw new Error(
        `[plugins] "${pluginId}" does not declare repository provider "${providerId}"`,
      );
    }
    const owner = this.providerOwners.get(providerId);
    if (owner && owner !== pluginId) {
      throw new Error(`[plugins] provider "${providerId}" is already owned by "${owner}"`);
    }
    this.providerOwners.set(providerId, pluginId);
  }

  private withRepositoryProviderLifecycle(
    pluginId: string,
    provider: RepositoryProviderRegistration,
  ): RepositoryProviderRegistration {
    return {
      ...provider,
      listRepositories: ({ workspaceId, signal }) =>
        this.runAbortable(pluginId, signal, (lifecycleSignal) =>
          provider.listRepositories({ workspaceId, signal: lifecycleSignal }),
        ),
      listBranches: ({ workspaceId, repository, signal }) =>
        this.runAbortable(pluginId, signal, (lifecycleSignal) =>
          provider.listBranches({ workspaceId, repository, signal: lifecycleSignal }),
        ),
      inspectURL: ({ workspaceId, url, signal }) =>
        this.runAbortable(pluginId, signal, (lifecycleSignal) =>
          provider.inspectURL({ workspaceId, url, signal: lifecycleSignal }),
        ),
    };
  }

  private withReviewProviderLifecycle(
    pluginId: string,
    provider: ReviewProviderRegistration,
  ): ReviewProviderRegistration {
    return {
      ...provider,
      getSnapshot: (taskId) => normalizeReviewItems(provider.id, provider.getSnapshot(taskId)),
      subscribe: (taskId, listener) =>
        this.trackReviewSubscription(pluginId, provider.subscribe(taskId, listener)),
      refresh: (taskId, signal) =>
        this.runAbortable(pluginId, signal, (lifecycleSignal) =>
          provider.refresh(taskId, lifecycleSignal),
        ),
    };
  }

  private runAbortable<T>(
    pluginId: string,
    sourceSignal: AbortSignal,
    operation: (signal: AbortSignal) => Promise<T>,
  ): Promise<T> {
    const controller = new AbortController();
    const controllers = this.abortControllersByPlugin.get(pluginId) ?? new Set<AbortController>();
    this.abortControllersByPlugin.set(pluginId, controllers);
    controllers.add(controller);
    const forwardAbort = () => controller.abort();
    if (sourceSignal.aborted) {
      forwardAbort();
    } else {
      sourceSignal.addEventListener("abort", forwardAbort, { once: true });
    }

    return Promise.resolve()
      .then(() => operation(controller.signal))
      .finally(() => {
        sourceSignal.removeEventListener("abort", forwardAbort);
        controllers.delete(controller);
        if (controllers.size === 0 && this.abortControllersByPlugin.get(pluginId) === controllers) {
          this.abortControllersByPlugin.delete(pluginId);
        }
      });
  }

  private trackReviewSubscription(pluginId: string, unsubscribe: () => void): () => void {
    const unsubscribers = this.reviewUnsubscribersByPlugin.get(pluginId) ?? new Set<() => void>();
    this.reviewUnsubscribersByPlugin.set(pluginId, unsubscribers);
    let closed = false;
    const trackedUnsubscribe = () => {
      if (closed) return;
      closed = true;
      unsubscribers.delete(trackedUnsubscribe);
      if (unsubscribers.size === 0) this.reviewUnsubscribersByPlugin.delete(pluginId);
      unsubscribe();
    };
    unsubscribers.add(trackedUnsubscribe);
    return trackedUnsubscribe;
  }

  private abortPluginWork(pluginId: string): void {
    this.abortControllersByPlugin.get(pluginId)?.forEach((controller) => controller.abort());
    this.abortControllersByPlugin.delete(pluginId);
    this.reviewUnsubscribersByPlugin.get(pluginId)?.forEach((unsubscribe) => unsubscribe());
    this.reviewUnsubscribersByPlugin.delete(pluginId);
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
      this.reviewProviders.size
    );
  }

  private notify(): void {
    this.version += 1;
    this.listeners.forEach((listener) => listener());
  }
}

function taskActionKey(pluginId: string, actionId: string): string {
  return `${pluginId}:${actionId}`;
}

function normalizeReviewItems(
  providerId: string,
  items: readonly ReviewItemSummary[],
): readonly ReviewItemSummary[] {
  return items.flatMap((item) => {
    if (
      item.providerId !== providerId ||
      !item.reviewKey ||
      !item.title ||
      !item.url ||
      !item.repositoryId ||
      !item.state
    ) {
      return [];
    }
    const statusBadge = item.statusBadge?.label
      ? {
          label: item.statusBadge.label,
          ...(item.statusBadge.tone ? { tone: item.statusBadge.tone } : {}),
        }
      : undefined;
    const taskStatus = normalizeReviewTaskStatus(item.taskStatus);
    return [
      {
        providerId,
        reviewKey: item.reviewKey,
        title: item.title,
        url: item.url,
        repositoryId: item.repositoryId,
        state: item.state,
        ...(statusBadge ? { statusBadge } : {}),
        ...(taskStatus ? { taskStatus } : {}),
      },
    ];
  });
}

const REVIEW_TASK_STATES = new Set<ReviewTaskStatus["state"]>([
  "open",
  "merged",
  "closed",
  "draft",
]);
const REVIEW_PIPELINE_STATES = new Set<ReviewTaskPipelineState>([
  "success",
  "failure",
  "pending",
  "neutral",
]);

function normalizeReviewTaskStatus(status: ReviewTaskStatus | undefined): ReviewTaskStatus | null {
  if (
    !status ||
    (typeof status.number !== "string" && typeof status.number !== "number") ||
    !REVIEW_TASK_STATES.has(status.state) ||
    !REVIEW_PIPELINE_STATES.has(status.pipelineState) ||
    !Array.isArray(status.checks)
  ) {
    return null;
  }
  const checks = status.checks.flatMap((check) => {
    if (!check.id || !check.label || !REVIEW_PIPELINE_STATES.has(check.state)) return [];
    return [
      {
        id: check.id,
        label: check.label,
        state: check.state,
        ...(check.detail ? { detail: check.detail } : {}),
        ...(check.url ? { url: check.url } : {}),
      },
    ];
  });
  const review = normalizeReviewTaskReview(status.review);
  return {
    number: status.number,
    state: status.state,
    pipelineState: status.pipelineState,
    checks,
    ...(review ? { review } : {}),
    ...(typeof status.unresolvedComments === "number" && status.unresolvedComments >= 0
      ? { unresolvedComments: status.unresolvedComments }
      : {}),
    ...(status.loading === true ? { loading: true } : {}),
    ...(status.error ? { error: status.error } : {}),
    ...(typeof status.updatedAt === "number" ? { updatedAt: status.updatedAt } : {}),
  };
}

function normalizeReviewTaskReview(review: ReviewTaskStatus["review"]) {
  if (
    !review ||
    !["approved", "changes_requested", "pending"].includes(review.state) ||
    !Number.isFinite(review.approved) ||
    review.approved < 0
  ) {
    return null;
  }
  return {
    state: review.state,
    approved: review.approved,
    ...(Number.isFinite(review.required) && (review.required ?? -1) >= 0
      ? { required: review.required }
      : {}),
    ...(Number.isFinite(review.requested) && (review.requested ?? -1) >= 0
      ? { requested: review.requested }
      : {}),
  };
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
