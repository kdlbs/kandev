import type { AppState } from "@/lib/state/store";
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

export type BootPayload = {
  version?: number;
  route?: BootRoute;
  runtime?: BootRuntime;
  initialState?: Partial<AppState>;
  routeData?: BootRouteData;
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

  return {
    version: typeof payload.version === "number" ? payload.version : undefined,
    route: isRecord(payload.route) ? readRoute(payload.route) : undefined,
    runtime,
    initialState: isRecord(payload.initialState) ? (payload.initialState as Partial<AppState>) : {},
    routeData: isRecord(payload.routeData) ? (payload.routeData as BootRouteData) : undefined,
    plugins: Array.isArray(payload.plugins) ? readPlugins(payload.plugins) : undefined,
    interimSettingsInterlockToken: readNonEmptyString(payload.interimSettingsInterlockToken),
  };
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
    lspAutoInstallPreferenceLanguages: readStringArray(value.lspAutoInstallPreferenceLanguages),
    debug: value.debug === true ? true : undefined,
    nonProduction: value.nonProduction === true ? true : undefined,
    locale: readString(value.locale),
    titlePrefix: readString(value.titlePrefix),
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
