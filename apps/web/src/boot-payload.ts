import type { HydrationState } from "@/lib/state/store";
import { getBackendConfig } from "@/lib/config";
import type { FetchedSessionData } from "@/lib/ssr/session-page-state";
import type {
  Repository,
  RepositoryBranchPolicy,
  RepositorySet,
  Task,
  Workflow,
  WorkflowStep,
} from "@/lib/types/http";
import type { ActivePlugin } from "@/lib/plugins/types";

export type { ActivePlugin };

export type BootRoute = {
  kind?: string;
  route?: string;
  path?: string;
  params?: Record<string, string>;
};

export type BootRuntime = {
  apiPrefix?: string;
  webSocketPath?: string;
  bootId?: string;
  lspAutoInstallPreferenceLanguages?: string[];
  debug?: boolean;
  /**
   * True for a dev or e2e build. The e2e harness serves a PRODUCTION bundle, so
   * `import.meta.env.PROD` cannot distinguish it from a real release — this flag
   * is what gates QA-only UI such as the pseudo-locale option.
   */
  nonProduction?: boolean;
  /** Active UI locale from the kandev_locale cookie; drives first-paint i18n. */
  locale?: string;
  /**
   * Operator-configured browser tab title prefix (KANDEV_WEB_TITLE_PREFIX), so
   * several Kandev instances are distinguishable in adjacent tabs. The Go shell
   * already rewrites `<title>`; this covers the /api/v1/app-state boot path,
   * which never renders through the shell.
   */
  titlePrefix?: string;
  /** True only when the Tauri shell launched the backend with its picker bridge. */
  nativeFolderPickerAvailable?: boolean;
  /** True when the backend was launched by the desktop shell. */
  desktopRuntime?: boolean;
};

export type BootRouteData = {
  taskDetail?: FetchedSessionData;
  routeContext?: {
    activeWorkspaceId?: string | null;
    workflows?: Workflow[];
    steps?: WorkflowStep[];
    repositories?: Repository[];
    repositorySets?: RepositorySet[];
    repositoryBranchPolicies?: RepositoryBranchPolicy[];
  };
  tasksPage?: {
    activeWorkspaceId?: string | null;
    workflows?: Workflow[];
    steps?: WorkflowStep[];
    repositories?: Repository[];
    repositorySets?: RepositorySet[];
    repositoryBranchPolicies?: RepositoryBranchPolicy[];
    tasks?: Task[];
    total?: number;
    tasksListSort?: string;
    tasksListGroup?: string;
  };
};

export type BootEntityGraph = {
  tasks?: Record<string, unknown>;
  sessions?: Record<string, unknown>;
};

export type BootPayload = {
  version?: number;
  route?: BootRoute;
  runtime?: BootRuntime;
  initialState?: HydrationState;
  routeData?: BootRouteData;
  entities?: BootEntityGraph;
  plugins?: ActivePlugin[];
  /** Replayable per-boot CSRF/accidental-mutation interlock; not authentication. */
  interimSettingsInterlockToken?: string;
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

  const version = typeof payload.version === "number" ? payload.version : undefined;
  const rawInitialState = isRecord(payload.initialState) ? payload.initialState : {};
  const rawRouteData = isRecord(payload.routeData) ? payload.routeData : undefined;
  const entities = readEntityGraph(payload.entities);
  const normalized =
    version === 2 && entities
      ? expandBootEntityGraph(rawInitialState, rawRouteData, entities)
      : { initialState: rawInitialState, routeData: rawRouteData };

  return {
    version,
    route: isRecord(payload.route) ? readRoute(payload.route) : undefined,
    runtime,
    initialState: normalized.initialState as HydrationState,
    routeData: normalized.routeData as BootRouteData | undefined,
    entities,
    plugins: Array.isArray(payload.plugins) ? readPlugins(payload.plugins) : undefined,
    interimSettingsInterlockToken: readNonEmptyString(payload.interimSettingsInterlockToken),
  };
}

function readEntityGraph(value: unknown): BootEntityGraph | undefined {
  if (!isRecord(value)) return undefined;
  const tasks = isRecord(value.tasks) ? value.tasks : {};
  const sessions = isRecord(value.sessions) ? value.sessions : {};
  return { tasks, sessions };
}

function expandBootEntityGraph(
  initialState: Record<string, unknown>,
  routeData: Record<string, unknown> | undefined,
  entities: BootEntityGraph,
) {
  return {
    initialState: expandBootNode(initialState, entities, ["initialState"]) as Record<
      string,
      unknown
    >,
    routeData: routeData
      ? (expandBootNode(routeData, entities, ["routeData"]) as Record<string, unknown>)
      : undefined,
  };
}

function expandBootNode(value: unknown, entities: BootEntityGraph, path: string[] = []): unknown {
  if (Array.isArray(value)) return value.map((entry) => expandBootNode(entry, entities, path));
  if (!isRecord(value)) return value;

  const result: Record<string, unknown> = {};
  for (const [key, child] of Object.entries(value)) {
    const reference = expandBootReference(key, child, path, entities);
    if (reference) result[reference.key] = reference.value;
    else result[key] = expandBootNode(child, entities, [...path, key]);
  }
  return result;
}

function expandBootReference(
  key: string,
  value: unknown,
  path: string[],
  entities: BootEntityGraph,
): { key: string; value: unknown } | undefined {
  return (
    expandTaskReference(key, value, path, entities.tasks ?? {}) ??
    expandSessionReference(key, value, path.at(-1) ?? "", entities.sessions ?? {})
  );
}

function expandTaskReference(
  key: string,
  value: unknown,
  path: string[],
  tasks: Record<string, unknown>,
): { key: string; value: unknown } | undefined {
  if (key === "taskIds" && isBootTaskListPath(path) && Array.isArray(value)) {
    return { key: "tasks", value: expandTaskIDs(value, tasks) };
  }
  if (key === "taskId" && isBootTaskDetailPath(path) && typeof value === "string" && tasks[value]) {
    return { key: "task", value: tasks[value] };
  }
  return undefined;
}

function isBootTaskListPath(path: string[]): boolean {
  const initialStatePath =
    path.slice(0, 3).join(".") === "routeData.taskDetail.initialState" ? path.slice(2) : path;
  return (
    initialStatePath.join(".") === "initialState.kanban" ||
    path.join(".") === "routeData.tasksPage" ||
    (initialStatePath.length === 4 &&
      initialStatePath.slice(0, 3).join(".") === "initialState.kanbanMulti.snapshots")
  );
}

function isBootTaskDetailPath(path: string[]): boolean {
  return (
    path.join(".") === "routeData.taskDetail" ||
    path.join(".") === "routeData.taskDetail.sidebarTaskPage.entries"
  );
}

function expandSessionReference(
  key: string,
  value: unknown,
  parentKey: string,
  sessions: Record<string, unknown>,
): { key: string; value: unknown } | undefined {
  if (parentKey === "taskSessions" && key === "sessionIds" && Array.isArray(value)) {
    return { key: "items", value: expandSessionItems(value, sessions) };
  }
  if (parentKey === "taskSessionsByTask" && key === "sessionIdsByTask" && isRecord(value)) {
    return { key: "itemsByTaskId", value: expandSessionLists(value, sessions) };
  }
  return undefined;
}

function expandTaskIDs(ids: unknown[], tasks: Record<string, unknown>) {
  return ids.flatMap((id) => (typeof id === "string" && tasks[id] ? [tasks[id]] : []));
}

function expandSessionItems(ids: unknown[], sessions: Record<string, unknown>) {
  return Object.fromEntries(
    ids.flatMap((id) => (typeof id === "string" && sessions[id] ? [[id, sessions[id]]] : [])),
  );
}

function expandSessionLists(value: Record<string, unknown>, sessions: Record<string, unknown>) {
  return Object.fromEntries(
    Object.entries(value).map(([taskId, ids]) => [
      taskId,
      Array.isArray(ids)
        ? ids.flatMap((id) => (typeof id === "string" && sessions[id] ? [sessions[id]] : []))
        : [],
    ]),
  );
}

export function readInterimSettingsInterlockToken(): string | undefined {
  if (typeof window === "undefined") return undefined;
  return readBootPayload(window).interimSettingsInterlockToken;
}

function readPlugins(value: unknown[]): ActivePlugin[] {
  return value.filter(isRecord).flatMap((entry) => {
    const plugin = readPlugin(entry);
    return plugin ? [plugin] : [];
  });
}

function readPlugin(value: Record<string, unknown>): ActivePlugin | undefined {
  const id = readString(value.id);
  const name = readString(value.name);
  const bundleUrl = readString(value.bundleUrl);
  if (!id || !name || !bundleUrl) return undefined;
  const styleUrls = Array.isArray(value.styleUrls)
    ? value.styleUrls.filter((entry): entry is string => typeof entry === "string")
    : undefined;
  const repositoryProviderIds = Array.isArray(value.repositoryProviderIds)
    ? value.repositoryProviderIds.filter((entry): entry is string => typeof entry === "string")
    : undefined;
  return {
    id,
    name,
    bundleUrl,
    styleUrls,
    ...(repositoryProviderIds ? { repositoryProviderIds } : {}),
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
    bootId: readNonEmptyString(value.bootId),
    lspAutoInstallPreferenceLanguages: readStringArray(value.lspAutoInstallPreferenceLanguages),
    debug: value.debug === true ? true : undefined,
    nonProduction: value.nonProduction === true ? true : undefined,
    locale: readString(value.locale),
    titlePrefix: readString(value.titlePrefix),
    nativeFolderPickerAvailable: value.nativeFolderPickerAvailable === true ? true : undefined,
    desktopRuntime: value.desktopRuntime === true ? true : undefined,
  };
}

function readString(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined;
}

function readStringArray(value: unknown): string[] | undefined {
  if (!Array.isArray(value) || !value.every((entry) => typeof entry === "string")) return undefined;
  return value;
}

function readNonEmptyString(value: unknown): string | undefined {
  const result = readString(value);
  return result ? result : undefined;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function isStringRecord(value: unknown): value is Record<string, string> {
  if (!isRecord(value)) return false;
  return Object.values(value).every((entry) => typeof entry === "string");
}
