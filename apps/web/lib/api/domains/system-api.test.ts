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
  fetchLogFiles,
  fetchLogTail,
  buildLogDownloadUrl,
  fetchUpdates,
  checkUpdates,
  applyUpdate,
  fetchSystemJob,
  fetchRestartCapability,
  requestRestart,
} from "./system-api";

const BASE = "http://api.test/api/v1/system";

type FetchInput = Parameters<typeof fetch>[0];
type FetchInit = Parameters<typeof fetch>[1];

const fetchSpy = vi.fn<[FetchInput, FetchInit?], Promise<Response>>();

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
  it("fetchLogFiles GETs /logs", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse([]));
    await fetchLogFiles();
    expect(lastCall().url).toBe(`${BASE}/logs`);
    expect(method()).toBe("GET");
  });

  it("fetchLogTail GETs /logs/tail with default n=1000 and no-store cache", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ lines: ["a", "b"] }));
    const res = await fetchLogTail();
    const { url, init } = lastCall();
    expect(url).toBe(`${BASE}/logs/tail?n=1000`);
    expect((init?.method ?? "GET").toUpperCase()).toBe("GET");
    expect(init?.cache).toBe("no-store");
    expect(res.lines).toEqual(["a", "b"]);
  });

  it("fetchLogTail uses the explicit n parameter when provided", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ lines: [] }));
    await fetchLogTail(250);
    expect(lastCall().url).toBe(`${BASE}/logs/tail?n=250`);
  });

  it("buildLogDownloadUrl returns the absolute download URL", () => {
    expect(buildLogDownloadUrl("kandev.log.1")).toBe(`${BASE}/logs/kandev.log.1/download`);
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
        started_at: "2026-05-18T00:00:00Z",
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
        latest_checked_at: "2026-05-18T00:00:00Z",
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
        latest_checked_at: "2026-05-18T00:00:00Z",
        update_available: false,
      }),
    );
    await checkUpdates();
    expect(lastCall().url).toBe(`${BASE}/updates/check`);
    expect(method()).toBe("POST");
  });

  it("applyUpdate POSTs /updates/apply with confirmation", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ job_id: "self-update-1" }));
    const res = await applyUpdate("UPDATE");
    const { url, init } = lastCall();
    expect(url).toBe(`${BASE}/updates/apply`);
    expect((init?.method ?? "").toUpperCase()).toBe("POST");
    expect(init?.body).toBe(JSON.stringify({ confirm: "UPDATE" }));
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
