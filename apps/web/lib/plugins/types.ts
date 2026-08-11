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
 * "task-sidebar", "settings-nav", "chat-input-actions"
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
 * "task-card-indicators" (small icon/badge rendered beside the PR status icon
 * on every kanban card — receives `{ taskId, workspaceId, workflowStepId }`
 * as `slotProps`), and "task-card-tags" (its own row on every kanban card,
 * rendered below the badges row — for contributions too wide for the cramped
 * title-row `task-card-indicators` spot, e.g. a row of tag chips — receives
 * `TaskCardTagsSlotProps` as `slotProps`). Not a closed union — hosts may
 * register additional slot names.
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

/** Context the host passes to the `task-card-tags` slot on every kanban card. */
export type TaskCardTagsSlotProps = {
  taskId: string;
  workspaceId: string | null;
  workflowStepId: string | null;
};

/** Component registered for a named slot; receives host-provided `slotProps`. */
export type SlotComponent = ReactType.ComponentType<{ slotProps?: unknown }>;

/** WS action payload handler registered by a plugin. */
export type WsHandler = (payload: unknown) => void;

/** Resource IDs plus untrusted JSON accepted by a declared plugin action. */
export interface PluginActionInput {
  workspaceId?: string;
  taskId?: string;
  /** Optional task session selector; host verifies task ownership before forwarding it. */
  sessionId?: string;
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

/** Stable read-only subset of a repository persisted by the Kandev host. */
export interface PluginHostRepository {
  id: string;
  workspace_id: string;
  name: string;
  provider: string;
  source_type?: string;
  provider_repo_id?: string;
  provider_host?: string;
  provider_owner?: string;
  provider_name?: string;
  remote_url?: string;
  default_branch?: string;
}

/** Provider-neutral branch descriptor consumed by Kandev's branch picker. */
export interface RepositoryProviderBranch {
  name: string;
}

/** Host context passed to a registered provider after the checkout branch is pushed. */
export interface RepositoryChangeRequestCreateContext {
  workspaceId: string;
  taskId: string;
  /** Session whose checkout Kandev pushed before invoking the provider. */
  sessionId: string;
  repositoryId: string;
  /** Persisted host repository; provider callbacks must not treat browser fields as authority. */
  repository: PluginHostRepository;
  title: string;
  body: string;
  baseBranch?: string;
  draft: boolean;
  signal: AbortSignal;
}

/** Minimal result adapted back into Kandev's native create-change-request feedback. */
export interface RepositoryChangeRequestCreateResult {
  url: string;
  provider?: string;
  output?: string;
  /** False means the remote change request exists but task association failed. */
  linked?: boolean;
  /** Safe, user-facing association failure detail. Creation must not be retried. */
  associationError?: string;
}

/** Repository-provider functions receive a host-managed cancellation signal. */
export interface RepositoryProviderRegistration {
  id: string;
  label: string;
  icon?: string;
  listRepositories(context: {
    workspaceId: string;
    signal: AbortSignal;
  }): Promise<RepositoryInspection[]>;
  matchesURL(url: string): boolean;
  listBranches(context: {
    workspaceId: string;
    repository: RepositoryInspection;
    signal: AbortSignal;
  }): Promise<RepositoryProviderBranch[]>;
  inspectURL(context: {
    workspaceId: string;
    url: string;
    signal: AbortSignal;
  }): Promise<RepositoryInspection | null>;
  /** False hides the native draft checkbox and forces `draft: false`. Default: true. */
  supportsDraft?: boolean;
  /** Called only after Kandev successfully pushes the verified checkout branch. */
  createChangeRequest?(
    context: RepositoryChangeRequestCreateContext,
  ): Promise<RepositoryChangeRequestCreateResult>;
}

/** Immutable current-task context supplied when a plugin action runs. */
export interface PluginTaskActionContext {
  workspaceId: string;
  taskId: string;
  repositories: readonly PluginHostRepository[];
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

/** Lightweight workspace link; rendered linked rows acquire a shared review-summary refresh. */
export interface ReviewTaskAssociation {
  providerId: string;
  taskId: string;
  reviewKey: string;
}

/** Verified UI context for removing one task-to-change-request association. */
export interface ReviewUnlinkContext {
  workspaceId: string;
  taskId: string;
  reviewKey: string;
  signal: AbortSignal;
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
  /** Optional bounded workspace association source for native task indicators. */
  getAssociationSnapshot?(workspaceId: string): readonly ReviewTaskAssociation[];
  subscribeAssociations?(workspaceId: string, listener: () => void): () => void;
  refreshAssociations?(workspaceId: string, signal: AbortSignal): Promise<void>;
  /** Removes only the selected task association; never deletes the remote review. */
  unlink?(context: ReviewUnlinkContext): Promise<void>;
  ReviewPanel: ReactType.ComponentType<PluginReviewPanelProps>;
  Selector?: ReactType.ComponentType;
  EmptyState?: ReactType.ComponentType;
}

/** Presentation context a task panel or kanban menu action renders under. */
export type PluginPresentation = "desktop" | "mobile";

/** Props passed to a `TaskPanelRegistration.Component`. */
export interface PluginTaskPanelProps {
  /** This registration's panel id, so one Component can back multiple panels. */
  panelId: string;
  taskId: string;
  sessionId: string | null;
  presentation: PluginPresentation;
}

/**
 * Registration accepted by `PluginRegistry.registerTaskPanel`: contributes a
 * dockview panel to the task workspace `+` (add panel) menu on desktop, and
 * (when `mobileEnabled`) the grouped Panels picker on a phone.
 */
export interface TaskPanelRegistration {
  /** Plugin-local panel id (unique within the plugin, not globally). */
  id: string;
  /** Add-panel-menu row label and dockview tab title. */
  title: string;
  /** Curated icon name (see `lib/plugins/icons.ts`). */
  icon?: string;
  /** Rendered as the panel body, wrapped in a `PluginErrorBoundary`. */
  Component: ReactType.ComponentType<PluginTaskPanelProps>;
  /** Include this panel in the phone's grouped Panels picker. Default: false. */
  mobileEnabled?: boolean;
}

/** Read-only context passed to `TaskMenuActionRegistration.visible`/`run`. */
export interface PluginTaskMenuContext {
  workspaceId: string;
  taskId: string;
  taskTitle: string;
  workflowStepId: string | null;
  presentation: PluginPresentation;
}

/**
 * Registration accepted by `PluginRegistry.registerTaskMenuAction`:
 * contributes an item to the kanban card context/dropdown menu. Group
 * "edit" nests the item inside the card's `Edit` submenu; group "primary"
 * renders it as a flat, top-level menu item, positioned between the "Move
 * to"/"Send to workflow" submenus and the "Link" submenu.
 */
export interface TaskMenuActionRegistration {
  id: string;
  label: string;
  icon?: ReactType.ReactNode;
  group: "edit" | "primary";
  /** Hide this item for a given card/context. Default: always visible. */
  visible?(context: PluginTaskMenuContext): boolean;
  /** A rejected promise is caught and logged; the menu still closes. */
  run(context: PluginTaskMenuContext): void | Promise<void>;
}

/** Read-only context passed to `TaskFilterRegistration.matches`. */
export interface PluginTaskFilterContext {
  taskId: string;
}

/** One selectable option returned by `TaskFilterRegistration.getOptions`. */
export interface PluginTaskFilterOption {
  value: string;
  label: string;
  /** Optional swatch color rendered beside the option label. */
  color?: string;
}

/** Stable host identity for a plugin task filter (`pluginId:id`). */
export type PluginTaskFilterRegistrationKey = `${string}:${string}`;

/**
 * Registration accepted by `PluginRegistry.registerTaskFilter`: contributes
 * a filter section to the kanban board's Workflow/Repository filter
 * dropdown. Filtering is evaluated client-side against tasks already loaded
 * in the board's in-memory state — there is no host-side pagination or
 * backend query for this filter.
 */
export interface TaskFilterRegistration {
  /** Plugin-local filter id; the host namespaces it as `pluginId:id`. */
  id: string;
  /** Filter section label shown in the dropdown. */
  label: string;
  /**
   * Options offered by this filter's multi-select, e.g. tag values. An
   * empty selection means "All" (no filtering applied) — the plugin decides
   * how to represent a sentinel like "untagged" as a normal option value; the
   * host does not special-case any option value.
   */
  getOptions(): PluginTaskFilterOption[];
  /**
   * Whether `context.taskId` should remain visible given the current
   * `selected` option values. Called only when `selected` is non-empty (an
   * empty selection always matches, without invoking this method).
   */
  matches(context: PluginTaskFilterContext, selected: string[]): boolean;
}

/** One entry returned by `host.storage.list`. */
export interface PluginStorageEntry {
  key: string;
  value: unknown;
  updatedAt: string;
}

/** Options accepted by `host.storage.set`/`delete`. */
export interface PluginStorageSetOptions {
  /** Cancels the request when the owning surface or plugin generation ends. */
  signal?: AbortSignal;
  /**
   * Optimistic-concurrency guard (approach H1): the `updatedAt` the caller
   * last read. The write is rejected (the returned promise rejects with a
   * `PluginStorageConflictError`) if the stored row was modified after this
   * time, leaving the stored value unchanged. Omit for unconditional
   * last-write-wins (today's default).
   */
  ifUnmodifiedSince?: string;
  /**
   * Identifies which logical surface made this write, for echo suppression
   * (see `subscribe`'s `writerId` filter). Omit to use the shared per-tab
   * default — fine for a one-shot write with no ongoing subscription (e.g. a
   * kanban menu action). A surface that also subscribes to its own writes
   * (e.g. a task panel) should pass something stable and unique to that
   * surface here, such as its own `panelId` from `PluginTaskPanelProps` —
   * otherwise its writes are indistinguishable from any other surface of the
   * same plugin in this tab, and one surface's legitimate write can be
   * silently swallowed by another surface's subscription as if it were that
   * other surface's own echo.
   */
  writerId?: string;
}

/** Per-user plugin storage scope — mirrors the backend's user-state routes. */
export type PluginStorageScope = "instance" | "workspace" | "task" | "session" | "repository";

/**
 * `host.storage` — authenticated, per-user key/value storage backed by
 * `/api/plugins/{id}/user-state/...`. Every read/write is scoped to the
 * calling user; two users writing the same (scope, scopeId, key) never see
 * each other's value. Requires the plugin manifest to declare
 * `capabilities.user_state: true`.
 */
export interface PluginStorageApi {
  get(
    scope: PluginStorageScope,
    scopeId: string,
    key: string,
    options?: { signal?: AbortSignal },
  ): Promise<PluginStorageEntry | undefined>;
  set(
    scope: PluginStorageScope,
    scopeId: string,
    key: string,
    value: unknown,
    options?: PluginStorageSetOptions,
  ): Promise<{ updatedAt: string }>;
  delete(
    scope: PluginStorageScope,
    scopeId: string,
    key: string,
    options?: Pick<PluginStorageSetOptions, "writerId" | "signal">,
  ): Promise<void>;
  /** Every entry under (scope, scopeId), ordered by key. Not paginated. */
  list(
    scope: PluginStorageScope,
    scopeId: string,
    options?: { signal?: AbortSignal },
  ): Promise<PluginStorageEntry[]>;
  /**
   * Subscribes to live updates for this plugin's own storage made from
   * another tab, device, or surface (approach F1) — e.g. the kanban `Edit`
   * modal and the task panel both editing the same document. `filter.scope`/
   * `scopeId`/`key` narrow to a specific tuple; omit a field to match any
   * value.
   *
   * `filter.writerId`, if given, must be the same value this surface passes
   * to `set`/`delete`'s own `writerId` option — a notification carrying that
   * exact id is this surface's own echo and is skipped (AC25), so its editor
   * never clobbers its own caret/selection reacting to its own write. Omit
   * to fall back to the shared per-tab default (today's behavior): correct
   * for a plugin with only one surface, but two independent surfaces of the
   * same plugin (e.g. an open task panel and a kanban quick-action) both
   * omitting it would incorrectly suppress each other's legitimate writes as
   * if they were one surface's own echo.
   */
  subscribe(
    filter: { scope?: PluginStorageScope; scopeId?: string; key?: string; writerId?: string },
    handler: (change: PluginUserStateChange) => void,
  ): () => void;
}

/** Payload delivered to a `host.storage.subscribe` handler. */
export interface PluginUserStateChange {
  scope: PluginStorageScope;
  scopeId: string;
  key: string;
  updatedAt: string;
  /** True when the change was a delete rather than a set. */
  deleted?: boolean;
}

/** Thrown by `host.storage.set` when `ifUnmodifiedSince` conflicts (HTTP 409). */
export class PluginStorageConflictError extends Error {
  constructor() {
    super("plugin storage: value was modified since ifUnmodifiedSince");
    this.name = "PluginStorageConflictError";
  }
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
  onSubmit(reference: string, signal: AbortSignal): Promise<void>;
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
   * Tabs, Dialog, Table, Popover, Progress, Accordion, Chart, ...) plus
   * first-party app UI (PageTopbar, TaskCreateDialog, RichTextEditor,
   * RichTextReadOnly). See `lib/plugins/host-api.ts` for the full list.
   */
  ui: Record<string, unknown>;
  /** Canonical responsive breakpoint hook for host-native plugin composition. */
  useResponsiveBreakpoint: typeof useResponsiveBreakpoint;
  /**
   * The resolved light/dark theme, read live on every access — a plugin's
   * `host` object is built once, so this must not be captured into a local
   * variable that outlives a render if you want it to stay current.
   * Components that read it during render re-read it on every re-render;
   * anything that paints imperatively (canvas, inline SVG colors) should pair
   * it with `onThemeChange`.
   */
  readonly theme: "light" | "dark";
  /**
   * Subscribes to light/dark changes — the settings picker, its live preview,
   * and an OS `prefers-color-scheme` flip while the app is on "system".
   * Returns an unsubscribe function; call it on teardown (component unmount,
   * `KandevPlugin.destroy`) or the listener outlives the surface that owns it.
   */
  onThemeChange(listener: (theme: "light" | "dark") => void): () => void;
  /** Soft SPA navigation (history push/replace + re-render), same as the app's router. */
  navigate(href: string, options?: { replace?: boolean }): void;
  /**
   * Imperatively opens a modal window rendered by the host's `<PluginModalHost/>`
   * (mounted once at the app root with its own tooltip provider). Independent of keybindings —
   * a keybinding handler may call it, but it works from any plugin code path.
   */
  openModal(options: PluginModalOptions): PluginModalHandle;
  /** Opens Kandev's native task change-request link flow with provider behavior. */
  openTaskLinkDialog(options: PluginTaskLinkDialogOptions): PluginModalHandle;
  /** Opens a provider-owned review in the task's native desktop or mobile surface. */
  openTaskReview(options: PluginTaskReviewOptions): void;
  /**
   * Sonner's imperative `toast`, scoped to this plugin. The host mounts the
   * single `<Toaster/>`, so there is nothing to render:
   * `host.toast.success(...)` / `.error(...)` / `.warning(...)` work from any
   * plugin code path, including inside a modal.
   *
   * `.error` additionally logs to the browser console as
   * `[plugins] toast.error from "<pluginId>":`. It does **not** file a report
   * into kandev's frontend error log — that log is for kandev's own
   * application errors, and a plugin toasting an expected condition (a failed
   * poll, say) would otherwise record one every cycle.
   */
  toast: PluginToastApi;
  /** Shared helpers — plain functions, not components. See `PluginUtilsApi`. */
  utils: PluginUtilsApi;
  /** Authenticated, per-user key/value storage. See `PluginStorageApi`. */
  storage: PluginStorageApi;
}

/**
 * `host.toast` — sonner's `toast`. Typed structurally rather than as
 * `typeof import("sonner").toast` so a plugin's own type-checking does not
 * need sonner installed.
 */
export type PluginToastApi = {
  (message: string, options?: Record<string, unknown>): string | number;
  success(message: string, options?: Record<string, unknown>): string | number;
  error(message: string, options?: Record<string, unknown>): string | number;
  warning(message: string, options?: Record<string, unknown>): string | number;
  info(message: string, options?: Record<string, unknown>): string | number;
  dismiss(id?: string | number): unknown;
};

/** `host.utils` — shared helpers a plugin would otherwise reimplement. */
export interface PluginUtilsApi {
  /** The host's `clsx` + `tailwind-merge` class combiner. */
  cn(...inputs: unknown[]): string;
  /** Non-security UUID with a fallback for supported insecure HTTP origins. */
  generateUUID(): string;
  /**
   * Locale-aware relative time ("3 hours ago", "in 2 days", "yesterday") via
   * `Intl.RelativeTimeFormat` in the user's active locale. Returns "" for
   * unparseable input. Prefer this over a hand-rolled ladder — those are
   * English-only and go untranslated for every non-English user.
   */
  formatRelativeTime(value: string | number | Date): string;
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
  /**
   * Contributes a panel to the task workspace `+` (add panel) menu (dockview
   * desktop) and, when `mobileEnabled`, the phone bottom nav. See
   * `TaskPanelRegistration`.
   */
  registerTaskPanel(registration: TaskPanelRegistration): void;
  /**
   * Contributes an item to the kanban card menu — nested in the `Edit`
   * submenu (group "edit") or as a flat top-level item (group "primary").
   * See `TaskMenuActionRegistration`.
   */
  registerTaskMenuAction(registration: TaskMenuActionRegistration): void;
  /**
   * Contributes a filter section to the kanban board's Workflow/Repository
   * filter dropdown. See `TaskFilterRegistration`.
   */
  registerTaskFilter(registration: TaskFilterRegistration): void;
}

/** Shape every plugin bundle registers via `window.registerKandevPlugin(id, plugin)`. */
export interface KandevPlugin {
  initialize(registry: PluginRegistry, host: PluginHostApi): void | Promise<void>;
  destroy?(): void;
}
