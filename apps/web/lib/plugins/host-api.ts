/**
 * Builds the `PluginHostApi` (docs/plans/plugins/PLUGIN-API.md) passed
 * into a plugin's `initialize(registry, host)`.
 */
import * as React from "react";
import type { StoreApi } from "zustand";
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
import { Checkbox } from "@kandev/ui/checkbox";
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
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Input } from "@kandev/ui/input";
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
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { Combobox } from "@/components/combobox";
import { RichTextEditor, RichTextReadOnly } from "@/components/editors/tiptap/rich-text-editor";
import { PageTopbar } from "@/components/page-topbar";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import { getBackendConfig } from "@/lib/config";
import { generateUUID } from "@/lib/utils";
import { softNavigate } from "@/lib/routing/client-router";
import type { AppState } from "@/lib/state/store";
import { pluginModalManager } from "./modal-manager";
import { composeWriterId, subscribeToUserStateChanges } from "./user-state-sync";
import {
  PluginStorageConflictError,
  type PluginHostApi,
  type PluginStorageApi,
  type PluginStorageEntry,
  type PluginStorageScope,
} from "./types";

/**
 * Curated `@kandev/ui` subset exposed on `host.ui`, plus a handful of
 * first-party app components (bottom of the map). Plugins must use these
 * host instances rather than bundling their own copies — bundling is not an
 * option for anything that touches React context or portals (Radix), since
 * the plugin shares the host React instance and a second copy would split
 * context and break refs/asChild. Pure-React libs (e.g. icon sets) bundle
 * fine.
 */
const PLUGIN_UI: Record<string, unknown> = {
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
  Checkbox,
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
  Input,
  Label,
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
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
  // - RichTextEditor / RichTextReadOnly: narrow wrappers over the Plan
  //   panel's tiptap editor (markdown paste, slash commands, mermaid),
  //   pixel-identical to the Plan panel. See rich-text-editor.tsx.
  RichTextEditor,
  RichTextReadOnly,
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

export function buildHostApi(
  pluginId: string,
  storeApi: StoreApi<AppState>,
  theme: "light" | "dark",
): PluginHostApi {
  return {
    pluginId,
    React,
    jsx: React.createElement,
    store: {
      getState: storeApi.getState,
      setState: storeApi.setState,
      subscribe: storeApi.subscribe,
    },
    api: {
      fetch: (path, init) => fetchPluginApi(pluginId, path, init),
      // Getter so split-origin dev/desktop always sees the current backend
      // origin, matching what fetchPluginApi resolves per call.
      get baseUrl() {
        return getBackendConfig().apiBaseUrl;
      },
    },
    ui: PLUGIN_UI,
    theme,
    navigate: (href, options) => softNavigate(href, options?.replace ? "replace" : "push"),
    openModal: (options) => pluginModalManager.openModal(pluginId, options),
    storage: buildStorageApi(pluginId),
  };
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
    async get(scope, scopeId, key) {
      const res = await fetchPluginApi(pluginId, userStatePath(scope, scopeId, key));
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
      });
      if (res.status === 409) throw new PluginStorageConflictError();
      if (!res.ok) throw new Error(`plugin storage: set failed with status ${res.status}`);
      const body = (await res.json()) as { updatedAt: string };
      return { updatedAt: body.updatedAt };
    },
    async delete(scope, scopeId, key, options) {
      const writerId = effectiveWriterId(options?.writerId);
      const path = `${userStatePath(scope, scopeId, key)}?writerId=${encodeURIComponent(writerId)}`;
      const res = await fetchPluginApi(pluginId, path, { method: "DELETE" });
      if (!res.ok) throw new Error(`plugin storage: delete failed with status ${res.status}`);
    },
    async list(scope, scopeId) {
      const res = await fetchPluginApi(pluginId, userStatePath(scope, scopeId));
      if (!res.ok) throw new Error(`plugin storage: list failed with status ${res.status}`);
      const body = (await res.json()) as { entries: PluginStorageEntry[] };
      return body.entries;
    },
    subscribe: (filter, handler) =>
      subscribeToUserStateChanges(pluginId, TAB_WRITER_ID, filter, handler),
  };
}
