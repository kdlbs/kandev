import type { AppState } from "@/lib/state/store";
import { getBackendConfig } from "@/lib/config";
import type { FetchedSessionData } from "@/lib/ssr/session-page-state";
import type { Repository, Task, Workflow, WorkflowStep } from "@/lib/types/http";

export type BootRoute = {
  kind?: string;
  route?: string;
  path?: string;
  params?: Record<string, string>;
};

export type BootRuntime = {
  apiPrefix?: string;
  webSocketPath?: string;
  debug?: boolean;
};

export type BootRouteData = {
  taskDetail?: FetchedSessionData;
  routeContext?: {
    activeWorkspaceId?: string | null;
    workflows?: Workflow[];
    steps?: WorkflowStep[];
    repositories?: Repository[];
  };
  tasksPage?: {
    activeWorkspaceId?: string | null;
    workflows?: Workflow[];
    steps?: WorkflowStep[];
    repositories?: Repository[];
    tasks?: Task[];
    total?: number;
    tasksListSort?: string;
    tasksListGroup?: string;
  };
};

export type BootPayload = {
  version?: number;
  route?: BootRoute;
  runtime?: BootRuntime;
  initialState?: Partial<AppState>;
  routeData?: BootRouteData;
};

type BootWindow = Window & {
  __KANDEV_BOOT_PAYLOAD__?: unknown;
  __KANDEV_DEBUG?: boolean;
};

export function readBootPayload(win: Window = window): BootPayload {
  const payload = (win as BootWindow).__KANDEV_BOOT_PAYLOAD__;
  if (!isRecord(payload)) return { initialState: {} };
  const runtime = isRecord(payload.runtime) ? readRuntime(payload.runtime) : undefined;
  if (runtime?.debug) {
    (win as BootWindow).__KANDEV_DEBUG = true;
  }

  return {
    version: typeof payload.version === "number" ? payload.version : undefined,
    route: isRecord(payload.route) ? readRoute(payload.route) : undefined,
    runtime,
    initialState: isRecord(payload.initialState) ? (payload.initialState as Partial<AppState>) : {},
    routeData: isRecord(payload.routeData) ? (payload.routeData as BootRouteData) : undefined,
  };
}

export async function loadBootPayload(
  win: Window = window,
  fetcher: typeof fetch = fetch,
): Promise<BootPayload> {
  const injected = (win as BootWindow).__KANDEV_BOOT_PAYLOAD__;
  if (isRecord(injected)) {
    return readBootPayload(win);
  }

  try {
    const path = `${win.location?.pathname || "/"}${win.location?.search || ""}`;
    const url = new URL(`${getBackendConfig().apiBaseUrl}/api/v1/app-state`);
    url.searchParams.set("path", path);
    const response = await fetcher(url.toString(), { cache: "no-store", credentials: "include" });
    if (!response.ok) return { initialState: {} };
    const payload = await response.json();
    (win as BootWindow).__KANDEV_BOOT_PAYLOAD__ = payload;
    return readBootPayload(win);
  } catch {
    return { initialState: {} };
  }
}

function readRoute(value: Record<string, unknown>): BootRoute {
  return {
    kind: readString(value.kind),
    route: readString(value.route),
    path: readString(value.path),
    params: isStringRecord(value.params) ? value.params : undefined,
  };
}

function readRuntime(value: Record<string, unknown>): BootRuntime {
  return {
    apiPrefix: readString(value.apiPrefix),
    webSocketPath: readString(value.webSocketPath),
    debug: value.debug === true ? true : undefined,
  };
}

function readString(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function isStringRecord(value: unknown): value is Record<string, string> {
  if (!isRecord(value)) return false;
  return Object.values(value).every((entry) => typeof entry === "string");
}
