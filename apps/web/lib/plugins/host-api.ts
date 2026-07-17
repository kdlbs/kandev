/**
 * Builds the `PluginHostApi` (docs/plans/plugins/PLUGIN-API.md) passed
 * into a plugin's `initialize(registry, host)`.
 */
import * as React from "react";
import type { StoreApi } from "zustand";
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
import { Separator } from "@kandev/ui/separator";
import { Skeleton } from "@kandev/ui/skeleton";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import { getBackendConfig } from "@/lib/config";
import type { AppState } from "@/lib/state/store";
import type { PluginHostApi } from "./types";

/** Curated `@kandev/ui` subset exposed on `host.ui`. */
const PLUGIN_UI: Record<string, unknown> = {
  Button,
  Badge,
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
  Separator,
  Skeleton,
  // First-party dialog: lets a plugin open kandev's real create-task modal,
  // prefilled via initialValues. Unlike the primitives above this is app UI,
  // not a shadcn primitive — exposed so plugins hand off task creation to the
  // native flow instead of POSTing directly.
  TaskCreateDialog,
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
    },
    ui: PLUGIN_UI,
    theme,
  };
}

/** fetch scoped to `/api/plugins/{pluginId}/...` via the kandev reverse proxy. */
function fetchPluginApi(pluginId: string, path: string, init?: RequestInit): Promise<Response> {
  const { apiBaseUrl } = getBackendConfig();
  const suffix = path.startsWith("/") ? path : `/${path}`;
  const url = `${apiBaseUrl}/api/plugins/${encodeURIComponent(pluginId)}${suffix}`;
  return fetch(url, init);
}
