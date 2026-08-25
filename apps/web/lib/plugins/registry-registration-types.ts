import type { ComponentType } from "react";
import type {
  IntegrationSettingsRegistration,
  NavItem,
  PluginRouteOptions,
  PluginTaskFilterRegistrationKey,
  RepositoryProviderRegistration,
  ReviewProviderRegistration,
  SlotComponent,
  TaskActionRegistration,
  TaskFilterRegistration,
  TaskListFacetRegistration,
  TaskMenuActionRegistration,
  TaskPanelRegistration,
} from "./types";

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

/** Nav item plus the owning pluginId. */
export interface PluginNavRegistration extends NavItem {
  pluginId: string;
}

/** Slot component plus its stable registry identity and owning plugin. */
export interface PluginSlotRegistration {
  registrationId: string;
  orderingId: string;
  pluginId: string;
  Component: SlotComponent;
}

/** Task panel registration plus the owning pluginId. */
export interface PluginTaskPanelRegistration extends TaskPanelRegistration {
  pluginId: string;
}

/** Task menu action registration plus the owning pluginId. */
export interface PluginTaskMenuActionRegistration extends TaskMenuActionRegistration {
  pluginId: string;
}

/** Task filter registration plus the owning pluginId. */
export interface PluginTaskFilterRegistration extends TaskFilterRegistration {
  pluginId: string;
}

/** Task-list facet registration paired with the plugin that owns it. */
export interface PluginTaskListFacetRegistration extends TaskListFacetRegistration {
  pluginId: string;
}

export function pluginTaskListFacetRegistrationKey(
  registration: Pick<PluginTaskListFacetRegistration, "pluginId" | "id">,
): string {
  return `${registration.pluginId}:${registration.id}`;
}

/** Stable UI/state identity for plugin-local task filter ids. */
export function pluginTaskFilterRegistrationKey(
  registration: Pick<PluginTaskFilterRegistration, "pluginId" | "id">,
): PluginTaskFilterRegistrationKey {
  return `${registration.pluginId}:${registration.id}`;
}

/** Host-owned lifecycle states used to reconcile registrations with UI state. */
export type PluginLifecycleStatus = "loading" | "ready" | "failed" | "removed";

/** A generation-fenced lifecycle snapshot; never exposed through PluginRegistry. */
export interface PluginLifecycleSnapshot {
  status: PluginLifecycleStatus;
  generation: number;
}
