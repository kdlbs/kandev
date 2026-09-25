import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import {
  getSSHReachability,
  getSSHExecutorReachability,
  probeSSHExecutorReachability,
} from "./ssh-api";
import type { SSHReachabilityRecord } from "@/lib/types/http-ssh";

const BASE = "http://api.test/api/v1/ssh";

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

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function lastCall(): { url: string; init: FetchInit | undefined } {
  const call = fetchSpy.mock.calls.at(-1);
  if (!call) throw new Error("expected fetch to have been called");
  return { url: String(call[0]), init: call[1] };
}

function record(overrides: Partial<SSHReachabilityRecord> = {}): SSHReachabilityRecord {
  return {
    executor_id: "exec-1",
    state: "reachable",
    reason: "",
    consecutive_failures: 0,
    host: "build.example",
    checked_at: "2026-09-17T00:00:00Z",
    last_success_at: "2026-09-17T00:00:00Z",
    updated_at: "2026-09-17T00:00:00Z",
    probing_enabled: true,
    probe_interval_seconds: 60,
    persisted: true,
    ...overrides,
  };
}

describe("getSSHReachability", () => {
  it("GETs the list route", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse([record()]));
    const result = await getSSHReachability();
    expect(result).toEqual([record()]);
    const { url, init } = lastCall();
    expect(url).toBe(`${BASE}/reachability`);
    expect(init?.method ?? "GET").toBe("GET");
  });
});

describe("getSSHExecutorReachability", () => {
  it("GETs the single-executor route", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse(record()));
    const result = await getSSHExecutorReachability("exec-1");
    expect(result).toEqual(record());
    const { url } = lastCall();
    expect(url).toBe(`${BASE}/executors/exec-1/reachability`);
  });

  it("encodes the executor id", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse(record({ executor_id: "exec/1" })));
    await getSSHExecutorReachability("exec/1");
    const { url } = lastCall();
    expect(url).toBe(`${BASE}/executors/exec%2F1/reachability`);
  });
});

describe("probeSSHExecutorReachability", () => {
  it("POSTs the probe-now route", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse(record()));
    const result = await probeSSHExecutorReachability("exec-1");
    expect(result).toEqual(record());
    const { url, init } = lastCall();
    expect(url).toBe(`${BASE}/executors/exec-1/reachability/probe`);
    expect(init?.method).toBe("POST");
  });
});
