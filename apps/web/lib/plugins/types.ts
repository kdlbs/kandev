/**
 * TS mirror of docs/plans/plugins/PLUGIN-API.md — the frozen contract.
 * Do not diverge without updating that document.
 */
import type * as ReactType from "react";
import type { StoreApi } from "zustand";
import type { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { AppState } from "@/lib/state/store";

/** Entry in the boot payload's `plugins` array (backend `ActivePlugin`). */
export interface ActivePlugin {
  id: string;
  name: string;
  bundleUrl: string;
  styleUrls?: string[];
  /** Manifest-owned repository provider IDs, supplied by newer boot payloads. */
  repositoryProviderIds?: string[];
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

/** Props supplied to a plugin-owned page inside Settings > Integrations. */
export interface PluginIntegrationSettingsProps {
  /** Explicit route workspace; omitted on the legacy global settings route. */
  workspaceId?: string;
}

/** Native integration-settings contribution owned by one active plugin. */
export interface IntegrationSettingsRegistration {
  /** URL-safe integration slug used under `/settings/.../integrations/{id}`. */
  id: string;
  label: string;
  description: string;
  /** Curated icon name from `lib/plugins/icons.ts`. */
  icon?: string;
  Component: ReactType.ComponentType<PluginIntegrationSettingsProps>;
}

/**
 * Named slot the host renders via `<PluginSlot name .../>`. Initial slots:
 * "task-sidebar", "settings-nav", "main-nav-footer", "chat-input-actions"
 * (icon buttons in the chat composer toolbar, beside the model picker / mic /
 * send — receives `{ taskId, taskTitle, activeSessionId, sessionIds }` as
 * `slotProps`), "chat-top-bar" (status in the session top bar, beside the
 * CPU/DB metrics — receives `{ taskId, taskTitle, workspaceId, activeSessionId,
 * sessionIds }`), "main-top-bar" (status/actions in the default app top bar on
 * the Home / Kanban / Tasks views, beside the CPU/DB metrics and the
 * view/display controls — the app-wide, task-agnostic counterpart to
 * "chat-top-bar"; receives `{ workspaceId, workspaceLabel, currentPage }`),
 * "app-status-bar-left" / "app-status-bar-right" (receives
 * `AppStatusBarSlotProps` as `slotProps`), and
 * "plugin-settings" (inline UI on a plugin's own settings
 * page, Settings > Plugins > <plugin>, at the top above the settings form —
 * receives `{ pluginId: string; status: PluginStatus }` as
 * `slotProps`). "plugin-settings" is owner-scoped: the host renders only the
 * component registered by the plugin being viewed, so a plugin never appears on
 * another plugin's page and authors don't gate on the current id themselves.
 * Not a closed union — hosts may register additional slot names.
 */
export type PluginSlotName = string;

/**
 * Context the host passes to every app status item. `placement` is its
 * registration/default side; a user's saved order may render it on the other side.
 */
export type AppStatusBarSlotProps = {
  placement: "left" | "right";
  presentation: "bar" | "mobile-drawer";
  density: "full" | "compact";
  pathname: string;
  activeWorkspaceId: string | null;
  activeTaskId: string | null;
  activeSessionId: string | null;
};

/** Component registered for a named slot; receives host-provided `slotProps`. */
export type SlotComponent = ReactType.ComponentType<{ slotProps?: unknown }>;

/** WS action payload handler registered by a plugin. */
export type WsHandler = (payload: unknown) => void;

/** Resource IDs plus untrusted JSON accepted by a declared plugin action. */
export interface PluginActionInput {
  workspaceId?: string;
  taskId?: string;
  repositoryId?: string;
  body?: unknown;
}

/** Transport controls for an authenticated plugin action. */
export interface PluginActionOptions {
  signal?: AbortSignal;
}

/** A provider-neutral repository/pull-request description returned by URL inspection. */
export interface RepositoryInspection {
  providerId: string;
  providerHost: string;
  ownerOrProject: string;
  repositoryId: string;
  repositoryName: string;
  cloneUrl: string;
  defaultBranch?: string;
  baseBranch?: string;
  headBranch?: string;
  pullRequest?: {
    number: number;
    title: string;
  };
}

/** Repository-provider functions receive a host-managed cancellation signal. */
export interface RepositoryProviderRegistration {
  id: string;
  label: string;
  icon?: string;
  listRepositories(context: { workspaceId: string; signal: AbortSignal }): Promise<unknown[]>;
  matchesURL(url: string): boolean;
  listBranches(context: {
    workspaceId: string;
    repository: unknown;
    signal: AbortSignal;
  }): Promise<unknown[]>;
  inspectURL(context: {
    workspaceId: string;
    url: string;
    signal: AbortSignal;
  }): Promise<RepositoryInspection | null>;
}

/** Immutable current-task context supplied when a plugin action runs. */
export interface PluginTaskActionContext {
  workspaceId: string;
  taskId: string;
  repositories: readonly unknown[];
  pathname: string;
  presentation: "desktop" | "mobile";
}

/** Native task-menu action supplied by a plugin. */
export interface TaskActionRegistration {
  id: string;
  label: string;
  icon?: string;
  placement: "link";
  group?: string;
  visible?(context: PluginTaskActionContext): boolean;
  singleTaskOnly?: boolean;
  run(context: PluginTaskActionContext): Promise<void>;
}

/** Normalized summary consumed by shared host review selectors and panels. */
export type ReviewTaskPipelineState = "success" | "failure" | "pending" | "neutral";

export interface ReviewTaskStatusCheck {
  id: string;
  label: string;
  state: ReviewTaskPipelineState;
  detail?: string;
  url?: string;
}

export interface ReviewTaskReviewSummary {
  state: "approved" | "changes_requested" | "pending";
  approved: number;
  required?: number;
  requested?: number;
}

/** Provider-neutral status rendered by the host in task topbar/composer chrome. */
export interface ReviewTaskStatus {
  number: number | string;
  state: "open" | "merged" | "closed" | "draft";
  pipelineState: ReviewTaskPipelineState;
  checks: readonly ReviewTaskStatusCheck[];
  review?: ReviewTaskReviewSummary;
  unresolvedComments?: number;
  loading?: boolean;
  error?: string;
  updatedAt?: number;
}

export interface ReviewItemSummary {
  providerId: string;
  reviewKey: string;
  title: string;
  url: string;
  repositoryId: string;
  state: string;
  statusBadge?: {
    label: string;
    tone?: string;
  };
  /** Optional task-chrome status; unknown/provider-specific keys are discarded by the registry. */
  taskStatus?: ReviewTaskStatus;
}

/** Props supplied to a provider-owned panel inside the host review surface. */
export interface PluginReviewPanelProps {
  panelId: string;
  presentation: "desktop" | "mobile";
  workspaceId: string;
  taskId: string;
  sessionId?: string;
  reviewKey: string;
}

/** External-store review provider; lifecycle-safe because it registers no React hooks. */
export interface ReviewProviderRegistration {
  id: string;
  label: string;
  icon?: string;
  changeRequestNoun: string;
  order: number;
  getSnapshot(taskId: string): readonly ReviewItemSummary[];
  subscribe(taskId: string, listener: () => void): () => void;
  refresh(taskId: string, signal: AbortSignal): Promise<void>;
  ReviewPanel: ReactType.ComponentType<PluginReviewPanelProps>;
  Selector?: ReactType.ComponentType;
  EmptyState?: ReactType.ComponentType;
}

/** Options accepted by `host.openModal(...)`. */
export interface PluginModalOptions {
  /** Modal title, rendered in a `DialogHeader`/`DialogTitle`. Omit to render no header title. */
  title?: string;
  /** Optional supporting copy rendered in the host-owned modal header. */
  description?: string;
  /** Component rendered inside the modal body — reuses the slot-component contract. */
  content: SlotComponent;
  /** Modal width, mapped to the host's Dialog size classes. Default: "md". */
  size?: "sm" | "md" | "lg" | "xl";
  /** Whether the modal can be dismissed via overlay click or Escape. Default: true. */
  dismissible?: boolean;
  /** Host-native presentation. Use `drawer` for phone/coarse-pointer task actions. */
  presentation?: "dialog" | "drawer";
}

/** Handle returned by `host.openModal(...)`, used to close that modal instance. */
export interface PluginModalHandle {
  /** Closes this modal instance. No-op if already closed. */
  close(): void;
}

/** Provider-owned copy and submit behavior for Kandev's native task-link dialog. */
export interface PluginTaskLinkDialogOptions {
  title: string;
  description: string;
  inputLabel: string;
  placeholder?: string;
  emptyError: string;
  failureMessage: string;
  successMessage: string;
  inputTestId?: string;
  errorTestId?: string;
  submitTestId?: string;
  onSubmit(reference: string): Promise<void>;
}

type PluginTaskReviewBaseOptions = {
  providerId: string;
  reviewKey: string;
  title: string;
};

/** Selects a provider-owned task review in the native desktop or mobile review surface. */
export type PluginTaskReviewOptions = PluginTaskReviewBaseOptions &
  ({ presentation: "desktop"; sessionId?: string } | { presentation: "mobile"; sessionId: string });

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
    /** Authenticated, manifest-declared browser action; never calls public webhooks. */
    invokeAction<TResponse>(
      key: string,
      input?: PluginActionInput,
      options?: PluginActionOptions,
    ): Promise<TResponse>;
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
  /** Canonical responsive breakpoint hook for host-native plugin composition. */
  useResponsiveBreakpoint: typeof useResponsiveBreakpoint;
  theme: "light" | "dark";
  /** Soft SPA navigation (history push/replace + re-render), same as the app's router. */
  navigate(href: string, options?: { replace?: boolean }): void;
  /**
   * Imperatively opens a modal window rendered by the host's `<PluginModalHost/>`
   * (mounted once inside the AppShell provider tree). Independent of keybindings —
   * a keybinding handler may call it, but it works from any plugin code path.
   */
  openModal(options: PluginModalOptions): PluginModalHandle;
  /** Opens Kandev's native task change-request link flow with provider behavior. */
  openTaskLinkDialog(options: PluginTaskLinkDialogOptions): PluginModalHandle;
  /** Opens a provider-owned review in the task's native desktop or mobile surface. */
  openTaskReview(options: PluginTaskReviewOptions): void;
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
  /** Native Settings > Integrations index, navigation, and detail contribution. */
  registerIntegrationSettings(integration: IntegrationSettingsRegistration): void;
  /**
   * Named slot injection, rendered by `<PluginSlot name .../>`. The
   * `app-status-bar-left` and `app-status-bar-right` receive
   * `AppStatusBarSlotProps` from the host.
   */
  registerComponent(slot: PluginSlotName, Component: SlotComponent): void;
  /** WS action handler, bridged into the existing `lib/ws` dispatch. */
  registerWsHandler(action: string, handler: WsHandler): void;
  /**
   * Bind a handler to a keybinding declared in this plugin's manifest
   * (`ui.keybindings[].id`). The host resolves the effective combo (user
   * override if set, else the manifest `default`) and dispatches globally,
   * skipping editable targets the same way core app shortcuts do. Binding an
   * `id` the manifest didn't declare still stores the handler (a console
   * warning is logged) since the dispatcher's effective-shortcut resolution
   * still keys off the manifest list.
   */
  registerKeybinding(id: string, handler: (event: KeyboardEvent) => void): void;
  /** Native repository discovery/inspection provider, revoked with this plugin. */
  registerRepositoryProvider(provider: RepositoryProviderRegistration): void;
  /** Native task-menu contribution, revoked with this plugin. */
  registerTaskAction(action: TaskActionRegistration): void;
  /** Native review-provider source and panel, revoked with this plugin. */
  registerReviewProvider(provider: ReviewProviderRegistration): void;
}

/** Shape every plugin bundle registers via `window.registerKandevPlugin(id, plugin)`. */
export interface KandevPlugin {
  initialize(registry: PluginRegistry, host: PluginHostApi): void | Promise<void>;
  destroy?(): void;
}
