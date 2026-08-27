/**
 * Host-runtime bindings for the public `@kandev/plugin-sdk` contract.
 * Provider-facing shapes come from the standalone SDK; this module adds only
 * concrete React/Zustand compatibility needed inside the Kandev web app.
 */
import type * as ReactType from "react";
import type { StoreApi } from "zustand";
import type { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { AppState } from "@/lib/state/store";
import type * as PluginSDK from "@kandev/plugin-sdk";
import type { PluginUIApi } from "@kandev/plugin-sdk";
export type {
  IntegrationSettingsActionProps,
  IntegrationSettingsActionSurface,
  PluginContextApi,
  PluginHostRepository,
  MainTopBarSlotProps,
  PluginNavSection,
  PluginUIApi,
} from "@kandev/plugin-sdk";
export type { PluginIcon } from "@kandev/plugin-sdk";

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
  /** Curated icon name or plugin-owned component; unknown names render the puzzle glyph. */
  icon?: PluginSDK.PluginIcon;
  /**
   * Where the item renders: "main" (default) as a top-level sidebar entry,
   * "integrations" inside the sidebar's Integrations section alongside the
   * first-party integration links, "sidebar-footer" as an icon button in the
   * sidebar footer's icon row and as a labelled row in the phone menu's
   * Utilities group (subject to the footer's inline budget — an over-budget
   * item is reached through the footer's overflow menu instead), "settings"
   * accepted but rendered on no surface.
   */
  section?: PluginSDK.PluginNavSection;
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
   * Curated icon name or plugin-owned component. Defaults to the matching nav
   * item's icon; unknown/missing names render no icon.
   */
  icon?: PluginSDK.PluginIcon;
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
  /** Curated icon name or plugin-owned component. */
  icon?: PluginSDK.PluginIcon;
  Component: ReactType.ComponentType<PluginIntegrationSettingsProps>;
  /** Optional action rendered in the detail section header and index card.
   * Receives the routed workspace and the native surface that mounted it. */
  action?: ReactType.ComponentType<PluginSDK.IntegrationSettingsActionProps>;
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
 * "chat-top-bar"; receives `{ workspaceId, workspaceLabel, currentPage,
 * presentation }`). On phones, `presentation` is "mobile": contributions
 * join the horizontally scrollable middle action strip between the fixed
 * Kandev link and menu button. Use the host `ui.Button` icon-button contract
 * there: a 32px box with a 16px SVG icon. Desktop contributions retain their
 * existing sizing.
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
 * as `slotProps`), "task-card-tags" (its own row on every kanban card,
 * rendered below the badges row — for contributions too wide for the cramped
 * title-row `task-card-indicators` spot, e.g. a row of tag chips — receives
 * `TaskCardTagsSlotProps` as `slotProps`), "task-row-metadata" (compact,
 * read-only secondary metadata on sidebar and `/tasks` rows — receives
 * `TaskRowMetadataSlotProps` as `slotProps`), and "sidebar-workspace-actions"
 * (icon-button cluster in the sidebar's New Task row, after the built-in
 * Quick Terminal and Quick Chat actions, with the same action group exposed
 * through mobile navigation — receives `SidebarWorkspaceActionsSlotProps`
 * as `slotProps`). Not a closed union — hosts may register additional slot
 * names.
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

/** Context passed to compact, plugin-agnostic task-row metadata contributions. */
export type TaskRowMetadataSlotProps = {
  taskId: string;
  workspaceId: string | null;
  workflowStepId: string | null;
  surface: "sidebar" | "task-list";
};

/** Context the host forwards to every `sidebar-workspace-actions` component. */
export type SidebarWorkspaceActionsSlotProps = {
  /** Active workspace for the current navigation surface. */
  workspaceId: string;
  /** Human-readable label of that workspace, when known. */
  workspaceLabel?: string;
  /** Desktop sidebar cluster or phone navigation action group. */
  presentation: "desktop" | "mobile";
};

/** Component registered for a named slot; receives host-provided `slotProps`. */
export type SlotComponent = ReactType.ComponentType<{ slotProps?: unknown }>;

/** WS action payload handler registered by a plugin. */
export type WsHandler = (payload: unknown) => void;

/** Resource IDs plus untrusted JSON accepted by a declared plugin action. */
export type PluginActionInput = PluginSDK.ActionInput;

/** Transport controls for an authenticated plugin action. */
export interface PluginActionOptions {
  signal?: AbortSignal;
}

/** A provider-neutral repository/pull-request description returned by URL inspection. */
export type RepositoryInspection = PluginSDK.RepositoryInspection;

/** Provider-neutral branch descriptor consumed by Kandev's branch picker. */
export type RepositoryProviderBranch = Awaited<
  ReturnType<PluginSDK.RepositoryProviderRegistration["listBranches"]>
>[number];

/** Host context passed to a registered provider after the checkout branch is pushed. */
export type RepositoryChangeRequestCreateContext = Parameters<
  NonNullable<PluginSDK.RepositoryProviderRegistration["createChangeRequest"]>
>[0];

/** Minimal result adapted back into Kandev's native create-change-request feedback. */
export type RepositoryChangeRequestCreateResult = Awaited<
  ReturnType<NonNullable<PluginSDK.RepositoryProviderRegistration["createChangeRequest"]>>
>;

export type RepositoryProviderPage = Extract<
  Awaited<ReturnType<PluginSDK.RepositoryProviderRegistration["listRepositories"]>>,
  { repositories: unknown }
>;

export type RepositoryProviderListResult = Awaited<
  ReturnType<PluginSDK.RepositoryProviderRegistration["listRepositories"]>
>;

/** Repository-provider functions receive a host-managed cancellation signal. */
export type RepositoryProviderRegistration = PluginSDK.RepositoryProviderRegistration;

/** Immutable current-task context supplied when a plugin action runs. */
export type PluginTaskActionContext = PluginSDK.TaskContext;

/** Native task-menu action supplied by a plugin. */
export type TaskActionRegistration = Parameters<PluginSDK.PluginRegistry["registerTaskAction"]>[0];

/** Normalized summary consumed by shared host review selectors and panels. */
export type ReviewTaskPipelineState = PluginSDK.ReviewTaskPipelineState;
export type ReviewTaskStatusCheck = PluginSDK.ReviewTaskStatusCheck;
export type ReviewTaskReviewSummary = NonNullable<PluginSDK.ReviewTaskStatus["review"]>;
/** Provider-neutral status rendered by the host in task topbar/composer chrome. */
export type ReviewTaskStatus = PluginSDK.ReviewTaskStatus;
export type ReviewItemSummary = PluginSDK.ReviewSummary;

/** Lightweight workspace link; rendered linked rows acquire a shared review-summary refresh. */
export type ReviewTaskAssociation = PluginSDK.ReviewTaskAssociation;

/** Verified UI context for removing one task-to-change-request association. */
export type ReviewUnlinkContext = Parameters<
  NonNullable<Parameters<PluginSDK.PluginRegistry["registerReviewProvider"]>[0]["unlink"]>
>[0];

/** Props supplied to a provider-owned panel inside the host review surface. */
export type PluginReviewPanelProps = Parameters<
  Parameters<PluginSDK.PluginRegistry["registerReviewProvider"]>[0]["ReviewPanel"]
>[0];

/** External-store review provider; lifecycle-safe because it registers no React hooks. */
export type ReviewProviderRegistration = Parameters<
  PluginSDK.PluginRegistry["registerReviewProvider"]
>[0];

/** Presentation context a task panel or kanban menu action renders under. */
export type PluginPresentation = "desktop" | "mobile";

export type PluginComposerSurface = "task-chat" | "quick-chat" | "task-create" | "new-session";

export type PluginComposerSubmitResult =
  | { status: "submitted" }
  | { status: "blocked"; reason?: string }
  | { status: "unavailable" };

export interface PluginComposerCapability {
  insertText(text: string): { status: "inserted" | "ignored" | "unavailable" };
  focus(): { status: "focused" | "unavailable" };
  submit(): Promise<PluginComposerSubmitResult>;
}

export interface PluginComposerSlotProps {
  surface: PluginComposerSurface;
  presentation: PluginPresentation;
  taskId: string | null;
  taskTitle?: string;
  activeSessionId: string | null;
  sessionIds: string[];
  disabled: boolean;
  submittable: boolean;
  disabledReason?: string;
  composer: PluginComposerCapability;
}

/** Props passed to a `TaskPanelRegistration.Component`. */
export type PluginTaskPanelProps = PluginSDK.PluginTaskPanelProps;

/**
 * Registration accepted by `PluginRegistry.registerTaskPanel`: contributes a
 * dockview panel to the task workspace `+` (add panel) menu on desktop, and
 * (when `mobileEnabled`) the grouped Panels picker on a phone.
 */
export type TaskPanelRegistration = PluginSDK.TaskPanelRegistration;

/** Read-only context passed to `TaskMenuActionRegistration.visible`/`run`. */
export interface PluginTaskMenuContext {
  /**
   * The active workspace id, or `""` when no workspace is active (e.g. the
   * Home / all-workspaces board, or a render before workspace hydration
   * completes) — the host always passes a string, never `null`/`undefined`,
   * so a plugin using this value as a `host.storage` scopeId must check for
   * `""` itself before calling `storage.get`/`set` (an empty scopeId fails
   * the backend's scopeId validation). This mirrors `TaskCardTagsSlotProps
   * .workspaceId`, which is `string | null` instead — a plugin reading both
   * shapes needs the same "no active workspace" guard for each.
   */
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
export type TaskMenuActionRegistration = PluginSDK.TaskMenuActionRegistration;

/** Read-only context passed to `TaskFilterRegistration.matches`. */
export type PluginTaskFilterContext = Parameters<PluginSDK.TaskFilterRegistration["matches"]>[0];

/** One selectable option returned by `TaskFilterRegistration.getOptions`. */
export type PluginTaskFilterOption = PluginSDK.PluginTaskFilterOption;

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
   * Omit this filter's section from the host's built-in display/filter
   * dropdown — for a plugin driving its own dedicated filter UI (e.g. a
   * top-bar dropdown) instead. The filter still gates cards exactly as
   * before via `matches`/`host.taskFilters`; only where its controls render
   * changes. Default: false (shown in the built-in dropdown, today's
   * behavior).
   */
  hidden?: boolean;
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

/** A plugin-provided, client-side facet for sorting and grouping `/tasks`. */
export type TaskListFacetRegistration = PluginSDK.TaskListFacetRegistration;
export type TaskListFacetValue = PluginSDK.TaskListFacetValue;

/** One entry returned by `host.storage.list`. */
export type PluginStorageEntry = PluginSDK.PluginStorageEntry;

/** Options accepted by `host.storage.set`/`delete`. */
export type PluginStorageSetOptions = NonNullable<Parameters<PluginSDK.PluginStorageApi["set"]>[4]>;

/** Per-user plugin storage scope — mirrors the backend's user-state routes. */
export type PluginStorageScope = PluginSDK.PluginStorageScope;

/** One entry returned by `host.storage.listByKey`, scanning across every scopeId for a fixed key. */
export interface PluginStorageScopeEntry {
  scopeId: string;
  value: unknown;
  updatedAt: string;
}

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
   * Every entry across every scopeId for a fixed (scope, key), ordered by
   * scopeId — e.g. every task carrying a given tag id. Backed by the
   * cross-scope scan route; capped server-side (`options.limit` requests a
   * smaller cap, never a larger one). `truncated` is true when more entries
   * exist than were returned.
   */
  listByKey(
    scope: PluginStorageScope,
    key: string,
    options?: { limit?: number },
  ): Promise<{ entries: PluginStorageScopeEntry[]; truncated: boolean }>;
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
export type PluginUserStateChange = PluginSDK.PluginUserStateChange;

/** Thrown by `host.storage.set` when `ifUnmodifiedSince` conflicts (HTTP 409). */
export class PluginStorageConflictError extends Error {
  constructor() {
    super("plugin storage: value was modified since ifUnmodifiedSince");
    this.name = "PluginStorageConflictError";
  }
}

/** Options accepted by `host.openModal(...)`. */
export type PluginModalOptions = Parameters<PluginSDK.PluginHostApi["openModal"]>[0];

/** Handle returned by `host.openModal(...)`, used to close that modal instance. */
export type PluginModalHandle = ReturnType<PluginSDK.PluginHostApi["openModal"]>;

/** Provider-owned copy and submit behavior for Kandev's native task-link dialog. */
export type PluginTaskLinkDialogOptions = Parameters<
  PluginSDK.PluginHostApi["openTaskLinkDialog"]
>[0];

/** Selects a provider-owned task review in the native desktop or mobile review surface. */
export type PluginTaskReviewOptions = Parameters<PluginSDK.PluginHostApi["openTaskReview"]>[0];

/**
 * API surface passed as the second argument to `KandevPlugin.initialize`.
 * Plugins must render with `host.React` / `host.jsx` — no bundled React.
 */
export type PluginHostApi = PluginSDK.PluginHostApi & {
  /** Concrete host React instance; public plugins consume its structural SDK type. */
  React: typeof ReactType;
  jsx: typeof ReactType.createElement;
  /** Legacy internal-store bridge. External plugins must use `host.context`. */
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
   * Tabs, Dialog, Table, Popover, Progress, Accordion, Chart, ...) plus
   * first-party app UI (PageTopbar, TaskCreateDialog, RichTextEditor,
   * RichTextReadOnly). See `lib/plugins/host-api.ts` for the full list.
   */
  ui: Record<string, unknown>;
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
   * (mounted once at the app root). Independent of keybindings — a keybinding
   * handler may call it, but it works from any plugin code path.
   */
  openModal(options: PluginModalOptions): PluginModalHandle;
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
  /**
   * Drives the selection for this plugin's own `registerTaskFilter`
   * registrations (typically paired with `hidden: true`, when the plugin
   * renders its own filter UI instead of the built-in dropdown section).
   * Namespaced internally to `${pluginId}:${id}` so a plugin can only ever
   * read/write its own filters' selections, never another plugin's.
   */
  taskFilters: PluginTaskFilterSelectionApi;
  /** Concrete Kandev components backing the SDK's curated structural UI map. */
  useResponsiveBreakpoint: typeof useResponsiveBreakpoint;
  useSettingsSaveContributor: PluginSDK.PluginHostApi["useSettingsSaveContributor"];
  setIntegrationEnabled: PluginSDK.PluginHostApi["setIntegrationEnabled"];
}

/**
 * `host.taskFilters` — lets a plugin's own UI (e.g. a top-bar dropdown) set
 * and observe the selection for its `registerTaskFilter` registrations,
 * the same selection state the built-in display/filter dropdown reads.
 */
export interface PluginTaskFilterSelectionApi {
  /** Current selected option values for this plugin's filter `id`, or `[]` if unset. */
  getSelection(id: string): string[];
  /** Empty `values` clears the selection (equivalent to "All"). */
  setSelection(id: string, values: string[]): void;
  /** Notified when any plugin task-filter selection changes; read this plugin's ids to detect relevant updates. */
  subscribe(listener: () => void): () => void;
}

/**
 * `host.toast` — sonner's `toast`. Typed structurally rather than as
 * `typeof import("sonner").toast` so a plugin's own type-checking does not
 * need sonner installed.
 */
export type PluginToastApi = PluginSDK.PluginToastApi;

export type PluginTranslationValues = PluginSDK.PluginTranslationValues;

export type PluginTranslationOptions = PluginSDK.PluginTranslationOptions;

export type PluginTranslationCatalogs = PluginSDK.PluginTranslationCatalogs;

export type PluginI18nApi = PluginSDK.PluginI18nApi;

/** `host.utils` — shared helpers a plugin would otherwise reimplement. */
export type PluginUtilsApi = PluginSDK.PluginHostApi["utils"];

/**
 * Registry surface passed as the first argument to `KandevPlugin.initialize`.
 * Each plugin receives an instance scoped to its own pluginId — the
 * registrations are tracked internally so the host can bulk-revoke them on
 * disable (see `apps/web/lib/plugins/registry.ts`).
 */
export type PluginRegistry = Omit<PluginSDK.PluginRegistry, "registerTaskFilter"> & {
  registerTaskFilter(registration: TaskFilterRegistration): void;
};

/** Shape every plugin bundle registers via `window.registerKandevPlugin(id, plugin)`. */
export interface KandevPlugin {
  initialize(registry: PluginRegistry, host: PluginHostApi): void | Promise<void>;
  destroy?(): void;
}
