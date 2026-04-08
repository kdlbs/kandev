import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { waitForHealth, waitForUrlReady } from "./health";

type ProcState = { exitCode: number | null };

const fetchMock = vi.fn();

describe("waitForUrlReady", () => {
  beforeEach(() => {
    fetchMock.mockReset();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("returns once web URL is reachable", async () => {
    const proc: ProcState = { exitCode: null };
    fetchMock
      .mockRejectedValueOnce(new Error("ECONNREFUSED"))
      .mockResolvedValue({ status: 404 } as Response);

    await expect(waitForUrlReady("http://localhost:3000", proc, 1000)).resolves.toBeUndefined();
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("fails when process exits before URL becomes reachable", async () => {
    const proc: ProcState = { exitCode: 1 };
    fetchMock.mockRejectedValue(new Error("ECONNREFUSED"));

    await expect(waitForUrlReady("http://localhost:3000", proc, 1000)).rejects.toThrow(
      "Web process exited before URL became reachable",
    );
  });

  it("fails on timeout when URL stays unreachable", async () => {
    const proc: ProcState = { exitCode: null };
    fetchMock.mockRejectedValue(new Error("ECONNREFUSED"));

    await expect(waitForUrlReady("http://localhost:3000", proc, 120)).rejects.toThrow(
      "Web URL readiness timed out",
    );
  });
});

describe("waitForHealth", () => {
  beforeEach(() => {
    fetchMock.mockReset();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("returns once /health responds ok", async () => {
    const proc: ProcState = { exitCode: null };
    fetchMock
      .mockRejectedValueOnce(new Error("ECONNREFUSED"))
      .mockResolvedValue({ ok: true } as Response);

    await expect(waitForHealth("http://localhost:8080", proc, 1000)).resolves.toBeUndefined();
  });

  it("includes the exit code and invokes onFailure when backend exits early", async () => {
    const proc: ProcState = { exitCode: 2 };
    fetchMock.mockRejectedValue(new Error("ECONNREFUSED"));
    const onFailure = vi.fn();

    await expect(waitForHealth("http://localhost:8080", proc, 1000, onFailure)).rejects.toThrow(
      "Backend exited (code 2) before healthcheck passed",
    );
    expect(onFailure).toHaveBeenCalledTimes(1);
  });

  it("timeout error mentions the health URL and the env override, and invokes onFailure", async () => {
    const proc: ProcState = { exitCode: null };
    fetchMock.mockRejectedValue(new Error("ECONNREFUSED"));
    const onFailure = vi.fn();

    await expect(waitForHealth("http://localhost:8080", proc, 120, onFailure)).rejects.toThrow(
      /http:\/\/localhost:8080\/health.*KANDEV_HEALTH_TIMEOUT_MS/s,
    );
    expect(onFailure).toHaveBeenCalledTimes(1);
  });
});
