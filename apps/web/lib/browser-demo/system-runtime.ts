import type { DemoHttpResponse } from "./protocol";
import type {
  DatabaseStats,
  StorageCapabilities,
  StorageMaintenanceRun,
  StorageMaintenanceSettings,
  StorageOverviewResponse,
  StorageQuarantineEntry,
  SystemJob,
} from "@/lib/types/system";

export type DemoSystemRouteContext = {
  path: string;
  method: string;
  input: Record<string, unknown>;
};

const NOW = "2026-07-18T12:00:00.000Z";
const GIB = 1024 ** 3;
const MIB = 1024 ** 2;

const DEFAULT_STORAGE_SETTINGS: StorageMaintenanceSettings = {
  enabled: true,
  check_interval_hours: 24,
  idle_for_minutes: 20,
  orphan_grace_hours: 72,
  quarantine_retention_hours: 168,
  workspaces: { enabled: true },
  kandev_containers: { enabled: true },
  go_cache: { enabled: true, max_bytes: 12 * GIB, adopted_path: "" },
  docker: {
    dedicated_daemon_acknowledged: true,
    build_cache_enabled: true,
    build_cache_keep_bytes: 8 * GIB,
    build_cache_unused_hours: 168,
    unused_images_enabled: false,
    unused_images_hours: 336,
  },
};

const STORAGE_CAPABILITIES: StorageCapabilities = {
  managed_go_cache_path: "/demo/.kandev/cache/go-build",
  go_cache_adoption_available: true,
  docker_available: true,
  docker_host: "unix:///var/run/docker.sock",
  host_global_docker_cleanup_allowed: false,
};

const INITIAL_QUARANTINE: StorageQuarantineEntry[] = [
  {
    id: "demo-quarantine-checkout",
    resource_type: "task_workspace",
    task_id: "demo-task-checkout-old-attempt",
    workspace_id: "demo-workspace",
    original_path: "/demo/worktrees/demo-task-checkout-old-attempt",
    quarantine_path: "/demo/.kandev/quarantine/demo-task-checkout-old-attempt",
    size_bytes: 286 * MIB,
    state: "quarantined",
    quarantined_at: "2026-07-17T09:30:00.000Z",
    delete_after: "2026-07-24T09:30:00.000Z",
    last_error: "",
    metadata: { reason: "orphaned task workspace", branch: "kandev/checkout-timeout-v1" },
  },
];

type DemoSystemState = {
  storageSettings: StorageMaintenanceSettings;
  quarantine: StorageQuarantineEntry[];
  databaseSize: number;
  walSize: number;
  jobSequence: number;
  jobs: Map<string, SystemJob>;
  runs: StorageMaintenanceRun[];
};

type SystemRoute = (
  context: DemoSystemRouteContext,
  state: DemoSystemState,
) => DemoHttpResponse | null;

const SYSTEM_ROUTES: SystemRoute[] = [
  routeCommandPreview,
  routeDiskUsage,
  routeDatabase,
  routeStorageOverview,
  routeStoragePolicy,
  routeStorageActions,
  routeStorageQuarantine,
  routeSystemJob,
];

export function createDemoSystemRuntime() {
  const storageSettings = structuredClone(DEFAULT_STORAGE_SETTINGS);
  const state: DemoSystemState = {
    storageSettings,
    quarantine: structuredClone(INITIAL_QUARANTINE),
    databaseSize: 18 * MIB + 384 * 1024,
    walSize: 768 * 1024,
    jobSequence: 0,
    jobs: new Map(),
    runs: [makeInitialStorageRun(storageSettings)],
  };
  return {
    route(context: DemoSystemRouteContext) {
      for (const router of SYSTEM_ROUTES) {
        const result = router(context, state);
        if (result) return result;
      }
      return null;
    },
  };
}

function routeCommandPreview({ path, method, input }: DemoSystemRouteContext) {
  const match = path.match(/^\/api\/v1\/agent-command-preview\/([^/]+)$/);
  return match && method === "POST"
    ? commandPreviewResponse(decodeURIComponent(match[1]), input)
    : null;
}

function routeDiskUsage({ path, method }: DemoSystemRouteContext, state: DemoSystemState) {
  if (path === "/api/v1/system/disk-usage" && method === "GET") {
    return ok(diskUsageResponse());
  }
  if (path === "/api/v1/system/disk-usage/refresh" && method === "POST") {
    return acceptJob(state, "disk-walk", { total_bytes: diskUsageResponse().data.total });
  }
  if (path === "/api/v1/system/disk-usage/open" && method === "POST") {
    return ok({ path: "/demo/.kandev" });
  }
  return null;
}

function routeDatabase({ path, method, input }: DemoSystemRouteContext, state: DemoSystemState) {
  if (path === "/api/v1/system/database" && method === "GET") {
    return ok(databaseStats(state.databaseSize, state.walSize));
  }
  if (path === "/api/v1/system/database/vacuum" && method === "POST") {
    const before = state.databaseSize;
    state.databaseSize -= 384 * 1024;
    state.walSize = 0;
    return acceptJob(state, "vacuum", {
      size_before: before,
      size_after: state.databaseSize,
      reclaimed_bytes: before - state.databaseSize,
    });
  }
  if (path === "/api/v1/system/database/optimize" && method === "POST") {
    return acceptJob(state, "optimize", { status: "ok" });
  }
  if (path !== "/api/v1/system/database/reset" || method !== "POST") return null;
  if (input.confirm !== "RESET") return error("Factory reset requires RESET confirmation", 400);
  return acceptJob(state, "factory-reset", {
    demo_mode: true,
    reset_performed: false,
    snapshot: "kandev-pre-reset-demo.db",
  });
}

function routeStorageOverview({ path, method }: DemoSystemRouteContext, state: DemoSystemState) {
  if (path === "/api/v1/system/storage" && method === "GET") {
    return ok(storageOverview(state.storageSettings, state.quarantine, state.runs[0] ?? null));
  }
  if (path === "/api/v1/system/storage/runs" && method === "GET") {
    return ok({ runs: state.runs });
  }
  if (path === "/api/v1/system/storage/quarantine" && method === "GET") {
    return ok({ entries: state.quarantine });
  }
  return null;
}

function routeStoragePolicy(
  { path, method, input }: DemoSystemRouteContext,
  state: DemoSystemState,
) {
  if (path === "/api/v1/system/storage/settings" && method === "PATCH") {
    if (!isStorageSettings(input.settings))
      return error("Valid storage settings are required", 400);
    state.storageSettings = structuredClone(input.settings);
    return ok({ settings: state.storageSettings });
  }
  if (path !== "/api/v1/system/storage/go-cache/adopt" || method !== "POST") return null;
  if (input.confirm !== "ADOPT" || typeof input.path !== "string" || !input.path) {
    return error("Go-cache adoption requires ADOPT confirmation", 400);
  }
  state.storageSettings = {
    ...state.storageSettings,
    go_cache: { ...state.storageSettings.go_cache, enabled: true, adopted_path: input.path },
  };
  return ok({ settings: state.storageSettings, capabilities: STORAGE_CAPABILITIES });
}

function routeStorageActions(
  { path, method, input }: DemoSystemRouteContext,
  state: DemoSystemState,
) {
  if (path === "/api/v1/system/storage/analyze" && method === "POST") {
    const run = addStorageRun(state, "analysis", "Storage analysis completed", {
      candidate_bytes: 928 * MIB,
    });
    return acceptJob(state, "storage-analysis", { run_id: run.id });
  }
  if (path === "/api/v1/system/storage/run" && method === "POST") {
    const resources = Array.isArray(input.resources) ? input.resources : [];
    const run = addStorageRun(state, "manual", "Storage maintenance completed", {
      resources,
      reclaimed_bytes: 642 * MIB,
    });
    return acceptJob(state, "storage-cleanup", { run_id: run.id });
  }
  return null;
}

function routeStorageQuarantine(
  { path, method, input }: DemoSystemRouteContext,
  state: DemoSystemState,
) {
  const restore = path.match(/^\/api\/v1\/system\/storage\/quarantine\/([^/]+)\/restore$/);
  if (restore && method === "POST") {
    const entry = state.quarantine.find((item) => item.id === decodeURIComponent(restore[1]));
    if (!entry) return error("Quarantine entry not found", 404);
    state.quarantine = state.quarantine.filter((item) => item.id !== entry.id);
    return ok({ entry: { ...entry, state: "restored" as const, restored_at: NOW } });
  }
  const remove = path.match(/^\/api\/v1\/system\/storage\/quarantine\/([^/]+)$/);
  if (!remove || method !== "DELETE") return null;
  if (input.confirm !== "DELETE") return error("Permanent deletion requires confirmation", 400);
  const id = decodeURIComponent(remove[1]);
  if (!state.quarantine.some((item) => item.id === id)) {
    return error("Quarantine entry not found", 404);
  }
  state.quarantine = state.quarantine.filter((item) => item.id !== id);
  return acceptJob(state, "storage-quarantine-delete", { quarantine_id: id });
}

function routeSystemJob({ path, method }: DemoSystemRouteContext, state: DemoSystemState) {
  const match = path.match(/^\/api\/v1\/system\/jobs\/([^/]+)$/);
  if (!match || method !== "GET") return null;
  const found = state.jobs.get(decodeURIComponent(match[1]));
  return found ? ok(found) : error("System job not found", 404);
}

function acceptJob(
  state: DemoSystemState,
  kind: string,
  result: Record<string, unknown>,
): DemoHttpResponse {
  const id = `demo-${kind}-${++state.jobSequence}`;
  state.jobs.set(id, {
    id,
    kind,
    state: "succeeded",
    message: `${humanizeKind(kind)} completed in browser demo mode`,
    result,
    started_at: NOW,
    ended_at: NOW,
  });
  return response({ job_id: id }, 202);
}

function addStorageRun(
  state: DemoSystemState,
  trigger: StorageMaintenanceRun["trigger"],
  message: string,
  result: Record<string, unknown>,
) {
  const run: StorageMaintenanceRun = {
    id: `demo-storage-run-${state.runs.length + 1}`,
    trigger,
    state: "succeeded",
    settings_snapshot: structuredClone(state.storageSettings),
    result,
    message,
    started_at: NOW,
    completed_at: NOW,
  };
  state.runs.unshift(run);
  return run;
}

function commandPreviewResponse(agentName: string, input: Record<string, unknown>) {
  if (agentName !== "mock") {
    return ok({ supported: false, command: [], command_string: "" });
  }
  const model = typeof input.model === "string" && input.model ? input.model : "demo-fast";
  const enabledFlags = Array.isArray(input.cli_flags)
    ? input.cli_flags.flatMap((entry) => {
        if (!entry || typeof entry !== "object") return [];
        const flag = entry as { enabled?: unknown; flag?: unknown };
        return flag.enabled && typeof flag.flag === "string" ? [flag.flag] : [];
      })
    : [];
  const command = ["mock-agent", "--model", model, ...enabledFlags, "{prompt}"];
  return ok({ supported: true, command, command_string: command.join(" ") });
}

function diskUsageResponse() {
  const dataDir = 812 * MIB;
  const worktrees = 2.4 * GIB;
  const repos = 1.1 * GIB;
  const sessions = 184 * MIB;
  const tasks = 42 * MIB;
  const quickChat = 8 * MIB;
  const backups = 620 * MIB;
  return {
    data: {
      data_dir: dataDir,
      worktrees,
      repos,
      sessions,
      tasks,
      quick_chat: quickChat,
      backups,
      total: dataDir + worktrees + repos + sessions + tasks + quickChat + backups,
      warnings: [],
      computed_at: NOW,
    },
    computing: false,
    home_dir: "/demo/.kandev",
  };
}

function databaseStats(sizeBytes: number, walSizeBytes: number): DatabaseStats {
  return {
    driver: "sqlite",
    path: "/demo/.kandev/kandev.db",
    size_bytes: sizeBytes,
    wal_size_bytes: walSizeBytes,
    schema_version: "v1.24.0",
    last_backup_at: "2026-07-18T08:15:00.000Z",
  };
}

function storageOverview(
  settings: StorageMaintenanceSettings,
  quarantine: StorageQuarantineEntry[],
  lastRun: StorageMaintenanceRun | null,
): StorageOverviewResponse {
  return {
    settings,
    capabilities: STORAGE_CAPABILITIES,
    summary: {
      workspaces: {
        total_bytes: 2.4 * GIB,
        active_bytes: 1.5 * GIB,
        candidate_bytes: 928 * MIB,
        warnings: [],
        available: true,
      },
      go_cache: {
        path: STORAGE_CAPABILITIES.managed_go_cache_path,
        size_bytes: 3.2 * GIB,
        owned: true,
        enabled: settings.go_cache.enabled,
        unmanaged_path: settings.go_cache.adopted_path || "/demo/.cache/go-build",
        unmanaged_size_bytes: settings.go_cache.adopted_path ? 0 : 1.1 * GIB,
        available: true,
      },
      quarantine: {
        count: quarantine.length,
        size_bytes: quarantine.reduce((total, entry) => total + entry.size_bytes, 0),
      },
      docker: {
        available: true,
        image_layer_bytes: 1.8 * GIB,
        build_cache_bytes: 740 * MIB,
        unused_image_bytes: 310 * MIB,
        managed_container_count: 3,
        managed_container_bytes: 486 * MIB,
        warnings: [],
      },
    },
    last_run: lastRun,
  };
}

function makeInitialStorageRun(settings: StorageMaintenanceSettings): StorageMaintenanceRun {
  return {
    id: "demo-storage-run-1",
    trigger: "scheduled",
    state: "succeeded",
    settings_snapshot: structuredClone(settings),
    result: { scanned_bytes: 5.8 * GIB, reclaimed_bytes: 416 * MIB },
    message: "Scheduled maintenance completed",
    started_at: "2026-07-18T03:00:00.000Z",
    completed_at: "2026-07-18T03:00:02.000Z",
  };
}

function isStorageSettings(value: unknown): value is StorageMaintenanceSettings {
  if (!value || typeof value !== "object") return false;
  const candidate = value as Partial<StorageMaintenanceSettings>;
  return (
    typeof candidate.enabled === "boolean" &&
    typeof candidate.check_interval_hours === "number" &&
    typeof candidate.go_cache === "object" &&
    typeof candidate.docker === "object"
  );
}

function humanizeKind(kind: string) {
  return kind.replaceAll("-", " ");
}

function ok(body: unknown) {
  return response(body, 200);
}

function error(message: string, status: number) {
  return response({ error: message, demo_mode: true }, status);
}

function response(body: unknown, status: number): DemoHttpResponse {
  return { status, headers: { "content-type": "application/json" }, body };
}
