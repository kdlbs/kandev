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
  /** Curated icon name (see `lib/plugins/icons.ts`); unknown names render the puzzle glyph. */
  icon?: string;
  /**
   * Where the item renders: "main" (default) as a top-level sidebar entry,
   * "integrations" inside the sidebar's Integrations section alongside the
   * first-party integration links.
   */
  section?: "main" | "settings" | "integrations";
}

/**
 * Configuration for the kandev-style title bar the host renders above a
 * plugin route. Every field is optional — an empty object gets the same
 * derived defaults as omitting options entirely.
 */
export interface PluginPageChrome {
  /**
   * Topbar title. Defaults to the plugin's nav-item label registered for the
   * same path, else the plugin's display name.
   */
  title?: string;
  /** Muted subtitle rendered next to the title. */
  subtitle?: string;
  /**
   * Curated icon name (same set as `NavItem.icon`). Defaults to the matching
   * nav item's icon; unknown/missing names render no icon.
   */
  icon?: string;
  /** Where the topbar back link navigates (host default: "/"). */
  backHref?: string;
  /** Label for the back link (host default: "Kandev"). */
  backLabel?: string;
  /**
   * Component rendered on the right side of the topbar — use for dynamic
   * page actions (buttons, filters). Rendered with the host React instance.
   */
  actions?: ReactType.ComponentType;
}

/** Options accepted by `PluginRegistry.registerRoute`. */
export interface PluginRouteOptions {
  /**
   * Kandev-style title bar above the page. Default: enabled with derived
   * title. Pass a `PluginPageChrome` to configure it, or `false` to render
   * the route full-bleed and own the chrome yourself (e.g. with
   * `host.ui.PageTopbar`).
   */
  topbar?: boolean | PluginPageChrome;
}

/**
 * Named slot the host renders via `<PluginSlot name .../>`. Initial slots:
 * "task-sidebar", "settings-nav", "main-nav-footer", and "chat-input-actions"
 * (icon buttons in the chat composer toolbar, beside the model picker / mic /
 * send — receives `{ taskId, taskTitle, activeSessionId, sessionIds }` as
 * `slotProps`). Not a closed union — hosts may register additional slot names.
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
    /**
     * Backend API origin ("" when the SPA and API share an origin). Lets a
     * plugin reach first-party kandev REST endpoints without re-deriving the
     * split-origin dev/desktop base URL from window internals.
     */
    baseUrl: string;
  };
  /**
   * Curated subset of `@kandev/ui` components (Button, Card, Badge, Input,
   * Tabs, Dialog, Table, ...) plus first-party app UI (PageTopbar,
   * TaskCreateDialog). See `lib/plugins/host-api.ts` for the full list.
   */
  ui: Record<string, unknown>;
  theme: "light" | "dark";
  /** Soft SPA navigation (history push/replace + re-render), same as the app's router. */
  navigate(href: string, options?: { replace?: boolean }): void;
}

/**
 * Registry surface passed as the first argument to `KandevPlugin.initialize`.
 * Each plugin receives an instance scoped to its own pluginId — the
 * registrations are tracked internally so the host can bulk-revoke them on
 * disable (see `apps/web/lib/plugins/registry.ts`).
 */
export interface PluginRegistry {
  /**
   * Top-level SPA route, e.g. "/jira". Exact-match against window.location
   * path. The host wraps the page in kandev chrome (title bar) by default —
   * configure or opt out via `options.topbar`.
   */
  registerRoute(
    path: string,
    Component: ReactType.ComponentType,
    options?: PluginRouteOptions,
  ): void;
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
