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
import { PageTopbar } from "@/components/page-topbar";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import { ChangeRequestList, ChangeRequestRow } from "@/components/integrations/change-request-list";
import { ChangeRequestDetail } from "@/components/integrations/change-request-detail";
import { IntegrationStartTaskMenu } from "@/components/integrations/integration-start-task-menu";
import { IntegrationListToolbar } from "@/components/integrations/integration-list-toolbar";
import { IntegrationScopeBar } from "@/components/integrations/presets-scope-bar-base";
import { TaskRowIndicator } from "@/components/integrations/task-row-indicator";
import { IntegrationChangeRequestStatus } from "@/components/integrations/integration-change-request-status";
import { IntegrationIcon } from "@/components/integrations/integration-icon";
import { TaskChangeRequestLinkForm } from "@/components/integrations/task-change-request-link-form";
import { getBackendConfig } from "@/lib/config";
import { fetchJson } from "@/lib/api/client";
import { softNavigate } from "@/lib/routing/client-router";
import type { AppState } from "@/lib/state/store";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { reviewItemId } from "@/components/task/review-selection";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { pluginModalManager } from "./modal-manager";
import type {
  PluginActionInput,
  PluginActionOptions,
  PluginHostApi,
  PluginModalHandle,
  PluginTaskLinkDialogOptions,
  PluginTaskReviewOptions,
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
  ChangeRequestList,
  ChangeRequestRow,
  ChangeRequestDetail,
  IntegrationStartTaskMenu,
  IntegrationListToolbar,
  IntegrationChangeRequestStatus,
  IntegrationScopeBar,
  IntegrationIcon,
  TaskRowIndicator,
};

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
    ui: PLUGIN_UI,
    useResponsiveBreakpoint,
    theme,
    navigate: (href, options) => softNavigate(href, options?.replace ? "replace" : "push"),
    openModal: (options) => pluginModalManager.openModal(pluginId, options),
    openTaskLinkDialog: (options) => openTaskLinkDialog(pluginId, options),
    openTaskReview: (options) => openTaskReview(storeApi, options),
  };
}

function openTaskReview(storeApi: StoreApi<AppState>, options: PluginTaskReviewOptions): void {
  if (options.presentation === "desktop") {
    useDockviewStore
      .getState()
      .addReviewPanel(options.providerId, options.reviewKey, options.title);
    return;
  }
  storeApi
    .getState()
    .setMobileSessionReview(
      options.sessionId,
      reviewItemId({ providerId: options.providerId, reviewKey: options.reviewKey }),
    );
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

/** fetch scoped to `/api/plugins/{pluginId}/...` via the kandev reverse proxy. */
function fetchPluginApi(pluginId: string, path: string, init?: RequestInit): Promise<Response> {
  const { apiBaseUrl } = getBackendConfig();
  const suffix = path.startsWith("/") ? path : `/${path}`;
  const url = `${apiBaseUrl}/api/plugins/${encodeURIComponent(pluginId)}${suffix}`;
  return fetch(url, init);
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
    ...(input?.repositoryId ? { repositoryId: input.repositoryId } : {}),
    ...(input && "body" in input ? { body: input.body } : {}),
  };
  return fetchJson<TResponse>(
    `/api/plugins/${encodeURIComponent(pluginId)}/actions/${encodeURIComponent(key)}`,
    { init: { method: "POST", body: JSON.stringify(payload), signal: options?.signal } },
  );
}
