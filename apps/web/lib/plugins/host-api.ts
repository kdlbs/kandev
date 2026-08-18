/**
 * Builds the `PluginHostApi` (docs/plans/plugins/PLUGIN-API.md) passed
 * into a plugin's `initialize(registry, host)`.
 */
import * as React from "react";
import { useTranslation as useI18nextTranslation } from "react-i18next";
import type { StoreApi } from "zustand";
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@kandev/ui/accordion";
import { Alert, AlertDescription, AlertTitle } from "@kandev/ui/alert";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@kandev/ui/card";
import {
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartStyle,
  ChartTooltip,
  ChartTooltipContent,
} from "@kandev/ui/chart";
import { Checkbox } from "@kandev/ui/checkbox";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerOverlay,
  DrawerPortal,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@kandev/ui/empty";
import { Input } from "@kandev/ui/input";
import { Kbd, KbdGroup } from "@kandev/ui/kbd";
import { Label } from "@kandev/ui/label";
import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
} from "@kandev/ui/pagination";
import {
  Popover,
  PopoverAnchor,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from "@kandev/ui/popover";
import { Progress } from "@kandev/ui/progress";
import { ScrollArea } from "@kandev/ui/scroll-area";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Separator } from "@kandev/ui/separator";
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@kandev/ui/sheet";
import { Skeleton } from "@kandev/ui/skeleton";
import { Spinner } from "@kandev/ui/spinner";
import { Switch } from "@kandev/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { Textarea } from "@kandev/ui/textarea";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@kandev/ui/utils";
import { Combobox } from "@/components/combobox";
import { RichTextEditor, RichTextReadOnly } from "@/components/editors/tiptap/rich-text-editor";
import { PageTopbar } from "@/components/page-topbar";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import { ChangeRequestList, ChangeRequestRow } from "@/components/integrations/change-request-list";
import type { ChangeRequestDetailProps } from "@/components/integrations/change-request-detail";
import { IntegrationStartTaskMenu } from "@/components/integrations/integration-start-task-menu";
import { IntegrationListToolbar } from "@/components/integrations/integration-list-toolbar";
import { IntegrationScopeBar } from "@/components/integrations/presets-scope-bar-base";
import { IntegrationSaveQueryDialog } from "@/components/integrations/integration-save-query-dialog";
import { IntegrationRepositoryFilter } from "@/components/integrations/integration-repository-filter";
import { IntegrationCursorPagination } from "@/components/integrations/integration-cursor-pagination";
import { TaskRowIndicator } from "@/components/integrations/task-row-indicator";
import { IntegrationChangeRequestStatus } from "@/components/integrations/integration-change-request-status";
import { IntegrationIcon } from "@/components/integrations/integration-icon";
import { TaskChangeRequestLinkForm } from "@/components/integrations/task-change-request-link-form";
import { IntegrationAuthStatusBanner } from "@/components/integrations/auth-status-banner";
import { DraftedIntegrationEnabledControl } from "@/components/integrations/drafted-integration-enabled-control";
import { SettingsSection } from "@/components/settings/settings-section";
import { SettingsCard } from "@/components/settings/settings-card";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { WorkspaceScopedSection } from "@/components/integrations/workspace-scoped-section";
import { INTEGRATION_STATUS_REFRESH_MS } from "@/hooks/domains/integrations/use-integration-availability";
import { getBackendConfig } from "@/lib/config";
import { fetchJson } from "@/lib/api/client";
import { i18n, normalizeLocale, t } from "@/lib/i18n";
import { formatRelativeTime } from "@/lib/i18n/formats";
import { createPluginToast } from "@/lib/toast/sonner";
import { generateUUID } from "@/lib/utils";
import { softNavigate } from "@/lib/routing/client-router";
import type { AppState } from "@/lib/state/store";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { reviewItemId } from "@/components/task/review-selection";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { pluginModalManager } from "./modal-manager";
import { pluginRegistry } from "./registry";
import { readResolvedTheme, subscribeToThemeChanges } from "./theme";
import { composeWriterId, subscribeToUserStateChanges } from "./user-state-sync";
import { buildPluginContextApi } from "./plugin-context-api";
import { pluginTranslationNamespace } from "./plugin-translations";
import type {
  PluginActionInput,
  PluginActionOptions,
  PluginHostApi,
  PluginI18nApi,
  PluginModalHandle,
  PluginTaskLinkDialogOptions,
  PluginTaskReviewOptions,
  PluginTranslationOptions,
} from "./types";
import type { PluginUIApi, SettingsSaveContributor } from "@kandev/plugin-sdk";
import {
  PluginStorageConflictError,
  type PluginStorageApi,
  type PluginStorageEntry,
  type PluginStorageScope,
} from "./types";

const LazyChangeRequestDetail = React.lazy(async () => {
  const module = await import("@/components/integrations/change-request-detail");
  return { default: module.ChangeRequestDetail };
});

/**
 * Keep Monaco and the markdown editor graph off the plugin boot path. The
 * shared detail view loads only when a plugin actually renders a review.
 */
function PluginChangeRequestDetail(props: ChangeRequestDetailProps) {
  return React.createElement(
    React.Suspense,
    {
      fallback: React.createElement(
        "div",
        { className: "flex h-full items-center justify-center py-8" },
        React.createElement(Spinner, {
          "aria-label": t("integrations:loadingChangeRequest"),
        }),
      ),
    },
    React.createElement(LazyChangeRequestDetail, props),
  );
}

/**
 * Curated `@kandev/ui` subset exposed on `host.ui`, plus a handful of
 * first-party app components (bottom of the map). Plugins must use these
 * host instances rather than bundling their own copies — bundling is not an
 * option for anything that touches React context or portals (Radix), since
 * the plugin shares the host React instance and a second copy would split
 * context and break refs/asChild. The same applies to recharts behind the
 * Chart* exports: it drives layout through its own context and portals
 * tooltips, so a bundled second copy splits that context exactly the way a
 * second Radix copy does. Pure-React libs (e.g. icon sets) bundle fine.
 */
const PLUGIN_UI: PluginUIApi & Record<string, unknown> = {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
  Alert,
  AlertDescription,
  AlertTitle,
  Badge,
  Button,
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
  // Chart: recharts wrappers. Plugins must use these rather than importing
  // recharts themselves — same context/portal hazard as Radix, and recharts
  // 2.15.4 is already a dependency of both apps/web and @kandev/ui, so using
  // the host's copy costs no bundle bytes.
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartStyle,
  ChartTooltip,
  ChartTooltipContent,
  Checkbox,
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerOverlay,
  DrawerPortal,
  DrawerTitle,
  DrawerTrigger,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
  Input,
  Kbd,
  KbdGroup,
  Label,
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
  Popover,
  PopoverAnchor,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
  Progress,
  ScrollArea,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Separator,
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
  Skeleton,
  Spinner,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
  Textarea,
  Tooltip,
  TooltipContent,
  // A plain Tooltip works everywhere a plugin renders through AppShell's
  // provider; PluginModalHost also keeps a local provider so isolated host
  // mounts stay safe. This is exported for plugins that want their own `delayDuration`/
  // `skipDelayDuration` for a dense cluster of tooltips — nesting a provider
  // is supported by Radix.
  TooltipProvider,
  TooltipTrigger,
  // App UI (not shadcn primitives), exposed so plugins compose kandev-native
  // surfaces instead of re-implementing them:
  // - Combobox: the app's Command+Popover picker (used by native toolbars).
  Combobox,
  // - PageTopbar: the first-party title bar. Plugin routes get one by default
  //   (registerRoute options.topbar); this export is for routes that opt out
  //   (`topbar: false`) and render their own chrome.
  PageTopbar,
  // - TaskCreateDialog: kandev's real create-task modal, prefilled via
  //   initialValues, so plugins hand off task creation to the native flow
  //   instead of POSTing directly.
  TaskCreateDialog,
  ChangeRequestList,
  ChangeRequestRow,
  ChangeRequestDetail: PluginChangeRequestDetail,
  IntegrationStartTaskMenu,
  IntegrationListToolbar,
  IntegrationChangeRequestStatus,
  IntegrationScopeBar,
  IntegrationSaveQueryDialog,
  // - IntegrationRepositoryFilter / IntegrationCursorPagination: the shared
  //   searchable provider filter and opaque-cursor footer for code-host pages.
  IntegrationRepositoryFilter,
  IntegrationCursorPagination,
  IntegrationIcon,
  TaskRowIndicator,
  // - RichTextEditor / RichTextReadOnly: narrow wrappers over the Plan
  //   panel's tiptap editor (markdown paste, slash commands, mermaid),
  //   pixel-identical to the Plan panel. See rich-text-editor.tsx.
  RichTextEditor,
  RichTextReadOnly,
  IntegrationAuthStatusBanner,
  IntegrationEnabledControl: DraftedIntegrationEnabledControl,
  SettingsSection,
  SettingsCard,
  WorkspaceScopedSection,
};

function pluginSettingsContributorId(pluginId: string, contributorId: string): string {
  return `plugin:${pluginId}:${contributorId}`;
}

function createPluginSettingsSaveContributorHook(pluginId: string) {
  return function usePluginSettingsSaveContributor(contributor: SettingsSaveContributor): void {
    useSettingsSaveContributor({
      ...contributor,
      id: pluginSettingsContributorId(pluginId, contributor.id),
    });
  };
}

function createPluginUIApi(pluginId: string): PluginUIApi & Record<string, unknown> {
  return {
    ...PLUGIN_UI,
    IntegrationEnabledControl: function PluginIntegrationEnabledControl(
      props: React.ComponentProps<typeof DraftedIntegrationEnabledControl>,
    ) {
      return React.createElement(DraftedIntegrationEnabledControl, {
        ...props,
        id: pluginSettingsContributorId(pluginId, props.id),
      });
    },
  };
}

/**
 * `host.utils` — plain functions, deliberately not on `host.ui` (a component
 * map). Shared rather than reimplemented per plugin: `cn` must be the host's
 * so Tailwind class merging matches the components it styles, and
 * `formatRelativeTime` must be the host's so a plugin's timestamps follow the
 * user's locale instead of hand-rolled English-only ladders. `generateUUID`
 * keeps non-security identifiers working when secure-context crypto APIs are
 * unavailable on a supported HTTP deployment.
 */
const PLUGIN_UTILS = {
  cn,
  formatRelativeTime,
  generateUUID,
  integrationStatusRefreshMs: INTEGRATION_STATUS_REFRESH_MS,
};

/**
 * Per-browser-tab id stamped on every `host.storage.set`/`delete` call and
 * echoed back in the `plugin.user-state.updated` WS payload, so
 * `host.storage.subscribe` can suppress a tab's own write (AC25) — one id
 * per page load, shared across every plugin in this tab (a subscription
 * already filters by pluginId, so a shared id doesn't cross plugin
 * boundaries).
 *
 * Uses `generateUUID` rather than `crypto.randomUUID` directly: the latter is
 * a secure-context-only API, so on an http:// non-localhost origin (a shared
 * VPS/homelab instance) it is undefined. This is module scope, so that would
 * throw during module init and take down plugin loading entirely — not just
 * storage.
 */
const TAB_WRITER_ID: string = generateUUID();

/**
 * Combines the per-tab base id with an optional per-surface discriminator
 * (`options.writerId`/`filter.writerId`) rather than replacing it outright.
 * A surface id like a dockview `panelId` is a static string derived from
 * `pluginId:panelKey` — identical across every browser tab/session that has
 * that same panel open. Using it as the *entire* writer id (in place of
 * `TAB_WRITER_ID`) would make two different tabs editing the same document
 * look like the same writer to each other, breaking cross-tab sync (AC24)
 * the same way the un-scoped shared id broke cross-surface sync. Appending
 * it to the tab-unique base instead keeps both properties: different tabs
 * always differ (the base differs), and different surfaces in the same tab
 * always differ (the discriminator differs).
 */
function effectiveWriterId(surfaceId: string | undefined): string {
  return composeWriterId(TAB_WRITER_ID, surfaceId);
}

function pluginTranslationValues(options?: PluginTranslationOptions): Record<string, unknown> {
  return {
    defaultValue: options?.defaultValue,
    count: options?.count,
    ...options?.values,
  };
}

function buildPluginI18nApi(pluginId: string): PluginI18nApi {
  const namespace = pluginTranslationNamespace(pluginId);
  return {
    get locale() {
      return normalizeLocale(i18n.resolvedLanguage ?? i18n.language);
    },
    t: (key, options) =>
      String(
        i18n.t(`${namespace}:${key}`, {
          ...pluginTranslationValues(options),
          defaultValue: options?.defaultValue ?? key,
        }),
      ),
    useTranslation() {
      const { i18n: activeI18n, t: translate } = useI18nextTranslation(namespace);
      const scopedTranslate = React.useCallback(
        (key: string, options?: PluginTranslationOptions) =>
          String(
            translate(key, {
              ...pluginTranslationValues(options),
              defaultValue: options?.defaultValue ?? key,
            }),
          ),
        [translate],
      );
      return {
        locale: normalizeLocale(activeI18n.resolvedLanguage ?? activeI18n.language),
        t: scopedTranslate,
      };
    },
  };
}

export function buildHostApi(pluginId: string, storeApi: StoreApi<AppState>): PluginHostApi {
  return {
    pluginId,
    React,
    jsx: React.createElement,
    i18n: buildPluginI18nApi(pluginId),
    store: {
      getState: storeApi.getState,
      setState: storeApi.setState,
      subscribe: storeApi.subscribe,
    },
    context: buildPluginContextApi(storeApi),
    api: {
      fetch: (path, init) => fetchPluginApi(pluginId, path, init),
      invokeAction: <TResponse>(
        key: string,
        input?: PluginActionInput,
        options?: PluginActionOptions,
      ) => invokePluginAction<TResponse>(pluginId, key, input, options),
      // Getter so split-origin dev/desktop always sees the current backend
      // origin, matching what fetchPluginApi resolves per call.
      get baseUrl() {
        return getBackendConfig().apiBaseUrl;
      },
    },
    ui: createPluginUIApi(pluginId),
    useResponsiveBreakpoint,
    // Getter, not a value captured at boot: a plugin built once at page load
    // would otherwise read the boot-time theme forever, and one that paints
    // imperatively (canvas, inline SVG colors) could never follow a
    // light/dark switch without a full reload.
    get theme() {
      return readResolvedTheme();
    },
    onThemeChange: (listener) => subscribeToThemeChanges(listener),
    navigate: (href, options) => softNavigate(href, options?.replace ? "replace" : "push"),
    openModal: (options) => pluginModalManager.openModal(pluginId, options),
    openTaskLinkDialog: (options) => openTaskLinkDialog(pluginId, options),
    openTaskReview: (options) => openTaskReview(storeApi, options),
    // Sonner's imperative global. AppShell mounts <Toaster/> once, so this
    // needs no host wiring and works from plugin modal content. Scoped per
    // plugin so `.error` logs with
    // attribution rather than filing a kandev frontend error report; see
    // createPluginToast.
    toast: createPluginToast(pluginId),
    utils: PLUGIN_UTILS,
    storage: buildStorageApi(pluginId),
    useSettingsSaveContributor: createPluginSettingsSaveContributorHook(pluginId),
    setIntegrationEnabled: (integrationId, workspaceId, enabled) =>
      pluginRegistry.setIntegrationEnabled(pluginId, integrationId, workspaceId, enabled),
  };
}

function openTaskReview(storeApi: StoreApi<AppState>, options: PluginTaskReviewOptions): void {
  if (options.presentation === "desktop") {
    useDockviewStore.getState().addReviewPanel(options);
    return;
  }
  storeApi.getState().setMobileSessionReview(options.sessionId, reviewItemId(options));
}

function openTaskLinkDialog(
  pluginId: string,
  options: PluginTaskLinkDialogOptions,
): PluginModalHandle {
  const handle = pluginModalManager.openTaskLinkDialog(pluginId, {
    title: options.title,
    description: options.description,
    presentation: "dialog",
    content: function PluginTaskLinkDialogContent() {
      return React.createElement(TaskChangeRequestLinkForm, {
        inputLabel: options.inputLabel,
        placeholder: options.placeholder,
        emptyError: options.emptyError,
        failureMessage: options.failureMessage,
        successMessage: options.successMessage,
        inputTestId: options.inputTestId,
        errorTestId: options.errorTestId,
        submitTestId: options.submitTestId,
        onSubmit: options.onSubmit,
        onCancel: () => handle.close(),
        onSuccess: () => handle.close(),
      });
    },
  });
  return handle;
}

/**
 * fetch scoped to `/api/plugins/{pluginId}/...` via the kandev reverse proxy.
 * Forces `credentials: "include"` so the session cookie still rides along
 * when the frontend and backend are on different origins/ports (split
 * dev/desktop setups) — plain `fetch` defaults to `same-origin` and would
 * silently drop it, turning every authenticated plugin request into a 401.
 */
function fetchPluginApi(pluginId: string, path: string, init?: RequestInit): Promise<Response> {
  const { apiBaseUrl } = getBackendConfig();
  const suffix = path.startsWith("/") ? path : `/${path}`;
  const url = `${apiBaseUrl}/api/plugins/${encodeURIComponent(pluginId)}${suffix}`;
  return fetch(url, { ...init, credentials: "include" });
}

/** Path for the per-user storage routes; omit `key` for the list route. */
function userStatePath(scope: PluginStorageScope, scopeId: string, key?: string): string {
  const base = `/user-state/${encodeURIComponent(scope)}/${encodeURIComponent(scopeId)}`;
  return key === undefined ? base : `${base}/${encodeURIComponent(key)}`;
}

/** Builds `host.storage` (`PluginStorageApi`), scoped to `pluginId`. */
function buildStorageApi(pluginId: string): PluginStorageApi {
  return {
    async get(scope, scopeId, key, options) {
      const res = await fetchPluginApi(pluginId, userStatePath(scope, scopeId, key), {
        signal: options?.signal,
      });
      if (res.status === 404) return undefined;
      if (!res.ok) throw new Error(`plugin storage: get failed with status ${res.status}`);
      const body = (await res.json()) as { value: unknown; updatedAt: string };
      return { key, value: body.value, updatedAt: body.updatedAt };
    },
    async set(scope, scopeId, key, value, options) {
      const res = await fetchPluginApi(pluginId, userStatePath(scope, scopeId, key), {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          value,
          writerId: effectiveWriterId(options?.writerId),
          ifUnmodifiedSince: options?.ifUnmodifiedSince,
        }),
        signal: options?.signal,
      });
      if (res.status === 409) throw new PluginStorageConflictError();
      if (!res.ok) throw new Error(`plugin storage: set failed with status ${res.status}`);
      const body = (await res.json()) as { updatedAt: string };
      return { updatedAt: body.updatedAt };
    },
    async delete(scope, scopeId, key, options) {
      const writerId = effectiveWriterId(options?.writerId);
      const path = `${userStatePath(scope, scopeId, key)}?writerId=${encodeURIComponent(writerId)}`;
      const res = await fetchPluginApi(pluginId, path, {
        method: "DELETE",
        signal: options?.signal,
      });
      if (!res.ok) throw new Error(`plugin storage: delete failed with status ${res.status}`);
    },
    async list(scope, scopeId, options) {
      const res = await fetchPluginApi(pluginId, userStatePath(scope, scopeId), {
        signal: options?.signal,
      });
      if (!res.ok) throw new Error(`plugin storage: list failed with status ${res.status}`);
      const body = (await res.json()) as { entries: PluginStorageEntry[] };
      return body.entries;
    },
    subscribe: (filter, handler) =>
      subscribeToUserStateChanges(pluginId, TAB_WRITER_ID, filter, handler),
  };
}

/** Calls the authenticated declared-action route; public webhook paths stay out of this API. */
function invokePluginAction<TResponse>(
  pluginId: string,
  key: string,
  input?: PluginActionInput,
  options?: PluginActionOptions,
): Promise<TResponse> {
  const payload = {
    ...(input?.workspaceId ? { workspaceId: input.workspaceId } : {}),
    ...(input?.taskId ? { taskId: input.taskId } : {}),
    ...(input?.sessionId ? { sessionId: input.sessionId } : {}),
    ...(input?.repositoryId ? { repositoryId: input.repositoryId } : {}),
    ...(input && "body" in input ? { body: input.body } : {}),
  };
  return fetchJson<TResponse>(
    `/api/plugins/${encodeURIComponent(pluginId)}/actions/${encodeURIComponent(key)}`,
    { init: { method: "POST", body: JSON.stringify(payload), signal: options?.signal } },
  );
}
