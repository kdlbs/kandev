import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Pin the backend config so URL assertions don't depend on the environment.
vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import {
  fetchSystemInfo,
  fetchDiskUsage,
  refreshDiskUsage,
  fetchDatabaseStats,
  vacuumDatabase,
  optimizeDatabase,
  resetDatabase,
  fetchBackups,
  createBackup,
  restoreBackup,
  deleteBackup,
  buildBackupDownloadUrl,
  buildDiagnosticBundleDownloadUrl,
  createDiagnosticBundle,
  fetchDiagnosticBundleCapabilities,
  fetchDiagnosticACPSessions,
  fetchDiagnosticBundle,
  uploadFrontendBundleChunk,
  fetchUpdates,
  checkUpdates,
  saveUpdatesChannel,
  applyUpdate,
  fetchSystemJob,
  fetchRestartCapability,
  requestRestart,
  adoptStorageGoCache,
  analyzeStorage,
  deleteStorageQuarantine,
  fetchStorageOverview,
  fetchStoragePolicy,
  fetchStorageQuarantine,
  fetchStorageRuns,
  purgeStorageQuarantine,
  restoreStorageQuarantine,
  runStorageMaintenance,
  saveStorageSettings,
} from "./system-api";

const BASE = "http://api.test/api/v1/system";
const ISO_CHECKED_AT = "2026-05-18T00:00:00Z";

type FetchInput = Parameters<typeof fetch>[0];
type FetchInit = Parameters<typeof fetch>[1];

const fetchSpy = vi.fn<(...args: [FetchInput, FetchInit?]) => Promise<Response>>();

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function jsonResponse(body: unknown, init?: ResponseInit): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
    ...init,
  });
}

function lastCall(): { url: string; init: FetchInit | undefined } {
  const call = fetchSpy.mock.calls.at(-1);
  if (!call) throw new Error("expected fetch to have been called");
  return { url: String(call[0]), init: call[1] };
}

function method(): string {
  return (lastCall().init?.method ?? "GET").toUpperCase();
}

describe("fetchSystemInfo", () => {
  it("GETs /info and returns the parsed body", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        version: "1.2.3",
        commit: "abc",
        build_time: "2026-01-01T00:00:00Z",
        go_version: "go1.24",
        os: "darwin",
        arch: "arm64",
      }),
    );
    const info = await fetchSystemInfo();
    expect(lastCall().url).toBe(`${BASE}/info`);
    expect(method()).toBe("GET");
    expect(info.version).toBe("1.2.3");
  });
});

describe("fetchDiskUsage", () => {
  it("GETs /disk-usage", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ data: null, computing: true }));
    const res = await fetchDiskUsage();
    expect(lastCall().url).toBe(`${BASE}/disk-usage`);
    expect(method()).toBe("GET");
    expect(res.computing).toBe(true);
  });
});

describe("refreshDiskUsage", () => {
  it("POSTs /disk-usage/refresh and returns the job id", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ job_id: "j-1" }));
    const res = await refreshDiskUsage();
    expect(lastCall().url).toBe(`${BASE}/disk-usage/refresh`);
    expect(method()).toBe("POST");
    expect(res.job_id).toBe("j-1");
  });
});

describe("fetchDatabaseStats", () => {
  it("GETs /database", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        driver: "sqlite",
        path: "/data/kandev.db",
        size_bytes: 1,
        wal_size_bytes: 0,
        schema_version: "1",
        last_backup_at: "",
      }),
    );
    const stats = await fetchDatabaseStats();
    expect(lastCall().url).toBe(`${BASE}/database`);
    expect(method()).toBe("GET");
    expect(stats.driver).toBe("sqlite");
    expect(stats.path).toBe("/data/kandev.db");
  });
});

describe("vacuumDatabase / optimizeDatabase", () => {
  it("vacuum POSTs /database/vacuum", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ job_id: "v-1" }));
    await vacuumDatabase();
    expect(lastCall().url).toBe(`${BASE}/database/vacuum`);
    expect(method()).toBe("POST");
  });

  it("optimize POSTs /database/optimize", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ job_id: "o-1" }));
    await optimizeDatabase();
    expect(lastCall().url).toBe(`${BASE}/database/optimize`);
    expect(method()).toBe("POST");
  });
});

describe("resetDatabase", () => {
  it("POSTs /database/reset with the confirm payload", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ job_id: "r-1" }));
    await resetDatabase("RESET");
    const { url, init } = lastCall();
    expect(url).toBe(`${BASE}/database/reset`);
    expect((init?.method ?? "").toUpperCase()).toBe("POST");
    expect(init?.body).toBe(JSON.stringify({ confirm: "RESET" }));
  });
});

describe("fetchBackups / createBackup / restoreBackup / deleteBackup", () => {
  it("fetchBackups GETs /backups", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse([]));
    const items = await fetchBackups();
    expect(lastCall().url).toBe(`${BASE}/backups`);
    expect(method()).toBe("GET");
    expect(items).toEqual([]);
  });

  it("createBackup POSTs /backups", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ job_id: "b-1" }));
    await createBackup();
    expect(lastCall().url).toBe(`${BASE}/backups`);
    expect(method()).toBe("POST");
  });

  it("restoreBackup POSTs /backups/:name/restore with body and url-encodes the name", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ job_id: "rs-1" }));
    await restoreBackup("manual 1.db", "RESTORE");
    const { url, init } = lastCall();
    expect(url).toBe(`${BASE}/backups/manual%201.db/restore`);
    expect((init?.method ?? "").toUpperCase()).toBe("POST");
    expect(init?.body).toBe(JSON.stringify({ confirm: "RESTORE" }));
  });

  it("deleteBackup DELETEs /backups/:name", async () => {
    fetchSpy.mockResolvedValueOnce(new Response(null, { status: 204 }));
    await deleteBackup("manual-1.db");
    const { url, init } = lastCall();
    expect(url).toBe(`${BASE}/backups/manual-1.db`);
    expect((init?.method ?? "").toUpperCase()).toBe("DELETE");
  });

  it("buildBackupDownloadUrl returns the absolute download URL", () => {
    expect(buildBackupDownloadUrl("manual 1.db")).toBe(`${BASE}/backups/manual%201.db/download`);
  });
});

describe("logs", () => {
  it("creates, inspects, uploads, and builds URLs for diagnostic bundles", async () => {
    fetchSpy
      .mockResolvedValueOnce(jsonResponse({ id: "bundle-1", status: "collecting" }))
      .mockResolvedValueOnce(jsonResponse({ id: "bundle-1", status: "ready" }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }));
    await createDiagnosticBundle(["backend", "frontend"]);
    expect(lastCall().url).toBe(`${BASE}/logs/bundles`);
    expect(method()).toBe("POST");
    expect(JSON.parse(String(lastCall().init?.body))).toEqual({
      sources: ["backend", "frontend"],
    });
    await fetchDiagnosticBundle("bundle-1");
    expect(lastCall().url).toBe(`${BASE}/logs/bundles/bundle-1`);
    expect(lastCall().init?.cache).toBe("no-store");
    await uploadFrontendBundleChunk("bundle-1", {
      browser_id: "browser",
      capture_stream_id: "stream",
      chunk_index: 0,
      done: true,
      storage_mode: "memory",
      capture_metadata: null,
      entries: [],
    });
    expect(lastCall().url).toBe(`${BASE}/logs/bundles/bundle-1/frontend`);
    expect(buildDiagnosticBundleDownloadUrl("bundle 1")).toBe(
      `${BASE}/logs/bundles/bundle%201/download`,
    );
  });

  it("sends selected ACP sessions and reads backend capabilities", async () => {
    fetchSpy
      .mockResolvedValueOnce(
        jsonResponse({
          id: "bundle-acp",
          status: "collecting",
          sources: ["acp"],
          session_ids: ["session-1"],
        }),
      )
      .mockResolvedValueOnce(
        jsonResponse({
          sources: ["backend", "frontend", "runtime", "acp"],
          acp_debug_enabled: true,
          acp_max_sessions: 10,
        }),
      )
      .mockResolvedValueOnce(
        jsonResponse({
          sessions: [
            {
              task_id: "task-1",
              task_title: "Repair diagnostic export",
              session_id: "session-1",
            },
          ],
        }),
      );
    await createDiagnosticBundle(["acp"], ["session-1"]);
    expect(JSON.parse(String(lastCall().init?.body))).toEqual({
      sources: ["acp"],
      session_ids: ["session-1"],
    });
    const capabilities = await fetchDiagnosticBundleCapabilities();
    expect(lastCall().url).toBe(`${BASE}/logs/capabilities`);
    expect(capabilities.acp_debug_enabled).toBe(true);
    const sessions = await fetchDiagnosticACPSessions();
    expect(lastCall().url).toBe(`${BASE}/logs/acp-sessions`);
    expect(sessions[0]?.session_id).toBe("session-1");
    expect(sessions[0]?.task_title).toBe("Repair diagnostic export");
  });
});

describe("fetchSystemJob", () => {
  it("GETs /jobs/:id and returns the job payload", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        id: "job-abc",
        kind: "vacuum",
        state: "succeeded",
        message: "done",
        started_at: ISO_CHECKED_AT,
      }),
    );
    const job = await fetchSystemJob("job-abc");
    expect(lastCall().url).toBe(`${BASE}/jobs/job-abc`);
    expect(method()).toBe("GET");
    expect(job.id).toBe("job-abc");
    expect(job.state).toBe("succeeded");
  });
});

describe("updates", () => {
  it("fetchUpdates GETs /updates", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        current: "1.0.0",
        latest: "1.0.1",
        latest_url: "https://gh/r",
        latest_checked_at: ISO_CHECKED_AT,
        update_available: true,
      }),
    );
    const res = await fetchUpdates();
    expect(lastCall().url).toBe(`${BASE}/updates`);
    expect(method()).toBe("GET");
    expect(res.update_available).toBe(true);
  });

  it("checkUpdates POSTs /updates/check", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        current: "1.0.0",
        latest: "1.0.0",
        latest_url: "",
        latest_checked_at: ISO_CHECKED_AT,
        update_available: false,
      }),
    );
    await checkUpdates();
    expect(lastCall().url).toBe(`${BASE}/updates/check`);
    expect(method()).toBe("POST");
  });

  it("saveUpdatesChannel PATCHes the typed channel without allowing init overrides", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        current: "1.0.0",
        latest: "1.0.1-nightly.shaabcdef123456",
        latest_url: "https://www.npmjs.com/package/kandev/v/1.0.1-nightly.shaabcdef123456",
        latest_checked_at: ISO_CHECKED_AT,
        update_available: true,
        channel: "nightly",
        channel_editable: true,
        channel_unsupported_reason: "",
      }),
    );

    const res = await saveUpdatesChannel("nightly", {
      init: { method: "GET", body: "caller override" },
    });
    const { url, init } = lastCall();

    expect(url).toBe(`${BASE}/updates/channel`);
    expect((init?.method ?? "").toUpperCase()).toBe("PATCH");
    expect(init?.body).toBe(JSON.stringify({ channel: "nightly" }));
    expect(res.channel).toBe("nightly");
  });

  it("applyUpdate POSTs /updates/apply with confirmation and the displayed target", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ job_id: "self-update-1" }));
    const res = await applyUpdate("UPDATE", "v1.0.1");
    const { url, init } = lastCall();
    expect(url).toBe(`${BASE}/updates/apply`);
    expect((init?.method ?? "").toUpperCase()).toBe("POST");
    expect(init?.body).toBe(JSON.stringify({ confirm: "UPDATE", target_version: "v1.0.1" }));
    expect(res.job_id).toBe("self-update-1");
  });
});

describe("restart", () => {
  it("fetchRestartCapability GETs /restart-capability", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ supported: false, mode: "manual" }));
    const res = await fetchRestartCapability();
    expect(lastCall().url).toBe(`${BASE}/restart-capability`);
    expect(method()).toBe("GET");
    expect(res.supported).toBe(false);
  });

  it("fetchRestartCapability always bypasses cache", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ supported: true, mode: "supervisor" }));
    await fetchRestartCapability({ cache: "force-cache" });
    expect(lastCall().init?.cache).toBe("no-store");
  });

  it("requestRestart POSTs /restart", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ accepted: true, message: "Restarting" }));
    const res = await requestRestart({ init: { method: "GET" } });
    expect(lastCall().url).toBe(`${BASE}/restart`);
    expect(method()).toBe("POST");
    expect(res.accepted).toBe(true);
  });
});

const storageSettings = {
  enabled: false,
  check_interval_hours: 24,
  idle_for_minutes: 10,
  orphan_grace_hours: 168,
  quarantine_retention_hours: 168,
  workspaces: { enabled: true },
  kandev_containers: { enabled: true },
  go_cache: { enabled: false, max_bytes: 16106127360, adopted_path: "" },
  docker: {
    dedicated_daemon_acknowledged: false,
    build_cache_enabled: false,
    build_cache_keep_bytes: 10737418240,
    build_cache_unused_hours: 168,
    unused_images_enabled: false,
    unused_images_hours: 168,
  },
};

describe("storage maintenance", () => {
  it("loads overview and list resources without caching", async () => {
    fetchSpy
      .mockResolvedValueOnce(
        jsonResponse({
          settings: storageSettings,
          summary: {},
          capabilities: {},
          analyzed_at: "2026-07-23T12:00:00Z",
          last_run: null,
        }),
      )
      .mockResolvedValueOnce(jsonResponse({ runs: [{ id: "run-1" }] }))
      .mockResolvedValueOnce(jsonResponse({ entries: [{ id: "entry-1" }] }));

    await fetchStorageOverview();
    expect(lastCall().url).toBe(`${BASE}/storage`);
    expect(lastCall().init?.cache).toBe("no-store");
    expect((await fetchStorageRuns())[0]?.id).toBe("run-1");
    expect(lastCall().url).toBe(`${BASE}/storage/runs?limit=20`);
    expect((await fetchStorageQuarantine())[0]?.id).toBe("entry-1");
  });

  it("saves dedicated Docker acknowledgement with its confirmation", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ settings: storageSettings }));
    await saveStorageSettings(storageSettings, "DEDICATED");
    expect(lastCall().url).toBe(`${BASE}/storage/settings`);
    expect(method()).toBe("PATCH");
    expect(JSON.parse(String(lastCall().init?.body))).toEqual({
      settings: storageSettings,
      confirmations: { dedicated_docker: "DEDICATED" },
    });
  });

  it("uses fixed confirmations for Go-cache adoption and permanent deletion", async () => {
    fetchSpy
      .mockResolvedValueOnce(jsonResponse({ settings: storageSettings, capabilities: {} }))
      .mockResolvedValueOnce(jsonResponse({ job_id: "delete-job" }));
    await adoptStorageGoCache("/root/.cache/go-build");
    expect(JSON.parse(String(lastCall().init?.body))).toEqual({
      path: "/root/.cache/go-build",
      confirm: "ADOPT",
    });
    const response = await deleteStorageQuarantine("entry/1");
    expect(lastCall().url).toBe(`${BASE}/storage/quarantine/entry%2F1`);
    expect(method()).toBe("DELETE");
    expect(JSON.parse(String(lastCall().init?.body))).toEqual({ confirm: "DELETE" });
    expect(response.job_id).toBe("delete-job");
  });

  it("uses typed confirmations for bulk quarantine purge", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ job_id: "eligible-job" }));
    await purgeStorageQuarantine("eligible");
    expect(lastCall().url).toBe(`${BASE}/storage/quarantine`);
    expect(method()).toBe("DELETE");
    expect(JSON.parse(String(lastCall().init?.body))).toEqual({
      scope: "eligible",
      confirm: "DELETE ELIGIBLE",
    });

    fetchSpy.mockResolvedValueOnce(jsonResponse({ job_id: "all-job" }));
    await purgeStorageQuarantine("all");
    expect(JSON.parse(String(lastCall().init?.body))).toEqual({
      scope: "all",
      confirm: "DELETE ALL NOW",
    });
  });

  it("starts analysis, selected cleanup, and restore operations", async () => {
    fetchSpy
      .mockResolvedValueOnce(jsonResponse({ job_id: "analysis" }))
      .mockResolvedValueOnce(jsonResponse({ job_id: "cleanup" }));
    expect((await analyzeStorage()).job_id).toBe("analysis");
    expect((await runStorageMaintenance(["workspaces"])).job_id).toBe("cleanup");
    expect(JSON.parse(String(lastCall().init?.body))).toEqual({ resources: ["workspaces"] });
    fetchSpy.mockResolvedValueOnce(jsonResponse({ job_id: "forced-cleanup" }));
    expect((await runStorageMaintenance(["workspaces"], true)).job_id).toBe("forced-cleanup");
    expect(JSON.parse(String(lastCall().init?.body))).toEqual({
      resources: ["workspaces"],
      force: true,
    });
    fetchSpy.mockResolvedValueOnce(jsonResponse({ entry: { id: "restored" } }));
    expect((await restoreStorageQuarantine("entry-1")).id).toBe("restored");
  });
});

describe("storage policy", () => {
  it("loads storage policy without requesting the scan-backed overview", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({ settings: storageSettings, capabilities: { docker_available: true } }),
    );

    const response = await fetchStoragePolicy();

    expect(lastCall().url).toBe(`${BASE}/storage/settings`);
    expect(lastCall().init?.cache).toBe("no-store");
    expect(response.settings).toEqual(storageSettings);
    expect(response.capabilities.docker_available).toBe(true);
  });
});
