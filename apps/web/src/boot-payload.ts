import type { AppState } from "@/lib/state/store";

export type BootRoute = {
  kind?: string;
  route?: string;
  path?: string;
  params?: Record<string, string>;
};

export type BootRuntime = {
  apiPrefix?: string;
  webSocketPath?: string;
};

export type BootPayload = {
  version?: number;
  route?: BootRoute;
  runtime?: BootRuntime;
  initialState?: Partial<AppState>;
};

type BootWindow = Window & {
  __KANDEV_BOOT_PAYLOAD__?: unknown;
};

export function readBootPayload(win: Window = window): BootPayload {
  const payload = (win as BootWindow).__KANDEV_BOOT_PAYLOAD__;
  if (!isRecord(payload)) return { initialState: {} };

  return {
    version: typeof payload.version === "number" ? payload.version : undefined,
    route: isRecord(payload.route) ? readRoute(payload.route) : undefined,
    runtime: isRecord(payload.runtime) ? readRuntime(payload.runtime) : undefined,
    initialState: isRecord(payload.initialState) ? (payload.initialState as Partial<AppState>) : {},
  };
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
