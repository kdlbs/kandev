import { describe, expect, it } from "vitest";
import type { StorageMaintenanceSettings, StorageOverviewResponse } from "@/lib/types/system";
import { createDemoSystemRuntime } from "./system-runtime";

function request(
  runtime: ReturnType<typeof createDemoSystemRuntime>,
  method: string,
  path: string,
  input: Record<string, unknown> = {},
) {
  const response = runtime.route({ method, path, input });
  expect(response).not.toBeNull();
  return response!;
}

describe("browser demo settings and system runtime", () => {
  it("previews the configured mock agent command", () => {
    const runtime = createDemoSystemRuntime();
    const response = request(runtime, "POST", "/api/v1/agent-command-preview/mock", {
      model: "demo-review",
      cli_passthrough: false,
      cli_flags: [
        { flag: "--verbose", description: "Verbose output", enabled: true },
        { flag: "--unsafe", description: "Unsafe mode", enabled: false },
      ],
    });

    expect(response).toMatchObject({
      status: 200,
      body: {
        supported: true,
        command: ["mock-agent", "--model", "demo-review", "--verbose", "{prompt}"],
        command_string: "mock-agent --model demo-review --verbose {prompt}",
      },
    });
  });

  it("serves disk and database data with pollable safe maintenance jobs", () => {
    const runtime = createDemoSystemRuntime();
    const disk = request(runtime, "GET", "/api/v1/system/disk-usage");
    const openFolder = request(runtime, "POST", "/api/v1/system/disk-usage/open");
    const refresh = request(runtime, "POST", "/api/v1/system/disk-usage/refresh");
    const refreshJobId = (refresh.body as { job_id: string }).job_id;
    const before = request(runtime, "GET", "/api/v1/system/database");
    const vacuum = request(runtime, "POST", "/api/v1/system/database/vacuum");
    const vacuumJobId = (vacuum.body as { job_id: string }).job_id;
    const after = request(runtime, "GET", "/api/v1/system/database");

    expect(disk).toMatchObject({
      status: 200,
      body: { computing: false, home_dir: "/demo/.kandev", data: { warnings: [] } },
    });
    expect(openFolder).toMatchObject({ body: { path: "/demo/.kandev" } });
    expect(request(runtime, "GET", `/api/v1/system/jobs/${refreshJobId}`)).toMatchObject({
      body: { kind: "disk-walk", state: "succeeded" },
    });
    expect(request(runtime, "GET", `/api/v1/system/jobs/${vacuumJobId}`)).toMatchObject({
      body: { kind: "vacuum", state: "succeeded" },
    });
    const optimize = request(runtime, "POST", "/api/v1/system/database/optimize");
    expect(
      request(
        runtime,
        "GET",
        `/api/v1/system/jobs/${(optimize.body as { job_id: string }).job_id}`,
      ),
    ).toMatchObject({ body: { kind: "optimize", state: "succeeded" } });
    expect((after.body as { size_bytes: number }).size_bytes).toBeLessThan(
      (before.body as { size_bytes: number }).size_bytes,
    );
    expect(request(runtime, "POST", "/api/v1/system/database/reset", {})).toMatchObject({
      status: 400,
    });
    expect(
      request(runtime, "POST", "/api/v1/system/database/reset", { confirm: "RESET" }),
    ).toMatchObject({ status: 202, body: { job_id: expect.any(String) } });
  });
});

describe("browser demo storage runtime", () => {
  it("serves a populated storage overview, run history, and quarantine", () => {
    const runtime = createDemoSystemRuntime();
    const overview = request(runtime, "GET", "/api/v1/system/storage");
    const runs = request(runtime, "GET", "/api/v1/system/storage/runs");
    const quarantine = request(runtime, "GET", "/api/v1/system/storage/quarantine");

    expect(overview).toMatchObject({
      status: 200,
      body: {
        settings: { enabled: true },
        capabilities: { docker_available: true, go_cache_adoption_available: true },
        summary: {
          workspaces: { candidate_bytes: expect.any(Number) },
          quarantine: { count: 1 },
          docker: { available: true },
        },
        last_run: { state: "succeeded" },
      },
    });
    expect(runs).toMatchObject({ body: { runs: [{ trigger: "scheduled" }] } });
    expect(quarantine).toMatchObject({
      body: { entries: [{ id: "demo-quarantine-checkout", state: "quarantined" }] },
    });
  });

  it("persists storage settings and completes demo-safe storage actions", () => {
    const runtime = createDemoSystemRuntime();
    const initial = request(runtime, "GET", "/api/v1/system/storage")
      .body as StorageOverviewResponse;
    const settings: StorageMaintenanceSettings = {
      ...initial.settings,
      check_interval_hours: 12,
    };

    expect(
      request(runtime, "PATCH", "/api/v1/system/storage/settings", { settings }),
    ).toMatchObject({ body: { settings: { check_interval_hours: 12 } } });
    expect(
      request(runtime, "POST", "/api/v1/system/storage/go-cache/adopt", {
        path: "/demo/.cache/go-build",
        confirm: "ADOPT",
      }),
    ).toMatchObject({
      body: { settings: { go_cache: { adopted_path: "/demo/.cache/go-build" } } },
    });

    for (const [path, kind] of [
      ["/api/v1/system/storage/analyze", "storage-analysis"],
      ["/api/v1/system/storage/run", "storage-cleanup"],
    ]) {
      const accepted = request(runtime, "POST", path);
      const id = (accepted.body as { job_id: string }).job_id;
      expect(request(runtime, "GET", `/api/v1/system/jobs/${id}`)).toMatchObject({
        body: { kind, state: "succeeded" },
      });
    }

    expect(
      request(
        runtime,
        "POST",
        "/api/v1/system/storage/quarantine/demo-quarantine-checkout/restore",
      ),
    ).toMatchObject({ body: { entry: { state: "restored" } } });
    expect(request(runtime, "GET", "/api/v1/system/storage/quarantine")).toMatchObject({
      body: { entries: [] },
    });
  });

  it("requires confirmation and safely completes quarantine deletion", () => {
    const runtime = createDemoSystemRuntime();
    const path = "/api/v1/system/storage/quarantine/demo-quarantine-checkout";

    expect(request(runtime, "DELETE", path, {})).toMatchObject({ status: 400 });
    const accepted = request(runtime, "DELETE", path, { confirm: "DELETE" });
    const id = (accepted.body as { job_id: string }).job_id;

    expect(accepted.status).toBe(202);
    expect(request(runtime, "GET", `/api/v1/system/jobs/${id}`)).toMatchObject({
      body: { kind: "storage-quarantine-delete", state: "succeeded" },
    });
    expect(request(runtime, "GET", "/api/v1/system/storage/quarantine")).toMatchObject({
      body: { entries: [] },
    });
  });
});
