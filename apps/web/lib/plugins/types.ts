/**
 * TS mirror of docs/plans/plugins/PLUGIN-API.md — the frozen contract.
 * Do not diverge without updating that document.
 */
import type * as ReactType from "react";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";

/** Entry in the boot payload's `plugins` array (backend `ActivePlugin`). */
export interface ActivePlugin {
  id: string;
  name: string;
  bundleUrl: string;
  styleUrls?: string[];
}

/** Sidebar/main nav entry registered by a plugin. */
export interface NavItem {
  id: string;
  label: string;
  path: string;
  icon?: string;
  section?: "main" | "settings";
}

/**
 * Named slot the host renders via `<PluginSlot name .../>`. Initial slots:
 * "task-sidebar", "settings-nav", "main-nav-footer". Not a closed union —
 * hosts may register additional slot names.
 */
export type PluginSlotName = string;

/** Component registered for a named slot; receives host-provided `slotProps`. */
export type SlotComponent = ReactType.ComponentType<{ slotProps?: unknown }>;

/** WS action payload handler registered by a plugin. */
export type WsHandler = (payload: unknown) => void;

/**
 * API surface passed as the second argument to `KandevPlugin.initialize`.
 * Plugins must render with `host.React` / `host.jsx` — no bundled React.
 */
export interface PluginHostApi {
  pluginId: string;
  /** Host React instance (shared) — plugins must not bundle their own React. */
  React: typeof ReactType;
  /** Convenience alias for `React.createElement`. */
  jsx: typeof ReactType.createElement;
  /** Kandev app store (zustand `StoreApi<AppState>`), curated to these 3 methods. */
  store: Pick<StoreApi<AppState>, "getState" | "setState" | "subscribe">;
  api: {
    /** fetch scoped to `/api/plugins/{id}/...` via the kandev reverse proxy. */
    fetch(path: string, init?: RequestInit): Promise<Response>;
  };
  /** Curated subset of `@kandev/ui` components (Button, Card, Badge, ...). */
  ui: Record<string, unknown>;
  theme: "light" | "dark";
}

/**
 * Registry surface passed as the first argument to `KandevPlugin.initialize`.
 * Each plugin receives an instance scoped to its own pluginId — the
 * registrations are tracked internally so the host can bulk-revoke them on
 * disable (see `apps/web/lib/plugins/registry.ts`).
 */
export interface PluginRegistry {
  /** Top-level SPA route, e.g. "/jira". Exact-match against window.location path. */
  registerRoute(path: string, Component: ReactType.ComponentType): void;
  /** Sidebar/main nav entry, rendered by `<PluginNavItems/>`. */
  registerNavItem(item: NavItem): void;
  /** Route under `/settings/plugins/{id}/...`, rendered inside the settings shell. */
  registerSettingsRoute(path: string, Component: ReactType.ComponentType): void;
  /** Named slot injection, rendered by `<PluginSlot name .../>`. */
  registerComponent(slot: PluginSlotName, Component: SlotComponent): void;
  /** WS action handler, bridged into the existing `lib/ws` dispatch. */
  registerWsHandler(action: string, handler: WsHandler): void;
}

/** Shape every plugin bundle registers via `window.registerKandevPlugin(id, plugin)`. */
export interface KandevPlugin {
  initialize(registry: PluginRegistry, host: PluginHostApi): void | Promise<void>;
  destroy?(): void;
}
